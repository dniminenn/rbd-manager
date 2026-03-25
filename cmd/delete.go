package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ceph/go-ceph/rbd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/spf13/cobra"
)

var forceFlag bool

var deleteCmd = &cobra.Command{
	Use:   "delete <image-name> [image-name...]",
	Short: "Delete one or more RBD images (interactive confirmation)",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runDelete,
}

func init() {
	deleteCmd.Flags().BoolVar(&forceFlag, "force", false, "Skip per-image confirmation (still requires final confirmation)")
	rootCmd.AddCommand(deleteCmd)
}

func prompt(reader *bufio.Reader, question string) bool {
	fmt.Print(question)
	answer, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(answer)) == "y"
}

func deletePV(cs *kubernetes.Clientset, pvName string) error {
	return cs.CoreV1().PersistentVolumes().Delete(context.Background(), pvName, metav1.DeleteOptions{})
}

func runDelete(cmd *cobra.Command, args []string) error {
	conn, err := connectCeph()
	if err != nil {
		return err
	}
	defer conn.Shutdown()

	// Open all pool ioctx upfront; we need the right one per image.
	poolMap, err := openPoolMap(conn)
	if err != nil {
		return err
	}
	defer func() {
		for _, ioctx := range poolMap {
			ioctx.Destroy()
		}
	}()

	cs, err := connectK8s()
	if err != nil {
		return err
	}

	pvImages, err := rbdImagesFromPVs(cs)
	if err != nil {
		return err
	}

	// Build a name->pool index from Ceph so we know which pool each image lives in.
	allImages, err := listAllImages(conn)
	if err != nil {
		return err
	}
	imagePool := make(map[string]string, len(allImages))
	for _, img := range allImages {
		imagePool[img.Name] = img.Pool
	}

	fmt.Println()
	cyan.Println("  Images scheduled for deletion:")
	fmt.Println()

	type target struct {
		name    string
		pool    string
		size    uint64
		pvPhase corev1.PersistentVolumePhase
		pvName  string
	}
	var targets []target

	for _, name := range args {
		p, ok := imagePool[name]
		if !ok {
			red.Fprintf(os.Stderr, "  x  %s: not found in any configured pool\n", name)
			continue
		}

		ioctx := poolMap[p]
		img, err := rbd.OpenImageReadOnly(ioctx, name, rbd.NoSnapshot)
		if err != nil {
			red.Fprintf(os.Stderr, "  x  %s: cannot open: %v\n", name, err)
			continue
		}
		size, _ := img.GetSize()
		img.Close()

		t := target{name: name, pool: p, size: size}
		if pv, ok := pvImages[name]; ok {
			t.pvPhase = pv.Phase
			t.pvName = pv.PVName
		}

		var status string
		switch t.pvPhase {
		case corev1.VolumeBound:
			status = red.Sprintf("BOUND  pv=%s", t.pvName)
		case corev1.VolumeReleased:
			status = yellow.Sprintf("Released  pv=%s", t.pvName)
		case "":
			status = red.Sprint("ORPHAN")
		default:
			status = yellow.Sprintf("%s  pv=%s", t.pvPhase, t.pvName)
		}

		fmt.Printf("  [%s]  %-44s  %9s  %s\n", dim.Sprint(p), white.Sprint(name), humanSize(size), status)
		targets = append(targets, t)
	}

	if len(targets) == 0 {
		return fmt.Errorf("no valid images")
	}

	var bound []string
	for _, t := range targets {
		if t.pvPhase == corev1.VolumeBound {
			bound = append(bound, t.name)
		}
	}
	if len(bound) > 0 {
		fmt.Println()
		red.Printf("  WARNING: %d image(s) have BOUND PersistentVolumes (actively in use).\n", len(bound))
		red.Println("  Deleting these WILL cause data loss and pod failures.")
	}

	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	if !forceFlag {
		var confirmed []target
		for _, t := range targets {
			var label string
			switch t.pvPhase {
			case corev1.VolumeBound:
				label = red.Sprint("BOUND")
			case corev1.VolumeReleased:
				label = yellow.Sprint("Released")
			case "":
				label = red.Sprint("ORPHAN")
			default:
				label = yellow.Sprint(string(t.pvPhase))
			}
			if !prompt(reader, fmt.Sprintf("  Delete %s (%s, %s)? [y/N] ", white.Sprint(t.name), humanSize(t.size), label)) {
				dim.Printf("  skipped %s\n", t.name)
				continue
			}

			// For Released images, offer to delete the PV first.
			if t.pvPhase == corev1.VolumeReleased {
				if prompt(reader, yellow.Sprintf("    PV %s is Released. Delete it now? [y/N] ", t.pvName)) {
					if err := deletePV(cs, t.pvName); err != nil {
						red.Fprintf(os.Stderr, "    x  failed to delete PV %s: %v\n", t.pvName, err)
						dim.Printf("  skipped %s\n", t.name)
						continue
					}
					green.Printf("    +  deleted PV %s\n", t.pvName)
				} else {
					dim.Printf("  skipped %s (PV not deleted)\n", t.name)
					continue
				}
			}

			confirmed = append(confirmed, t)
		}
		targets = confirmed
	}

	if len(targets) == 0 {
		dim.Println("\n  Nothing to delete.")
		return nil
	}

	fmt.Println()
	red.Printf("  About to permanently delete %d RBD image(s). This cannot be undone.\n", len(targets))
	fmt.Print("  Type 'delete' to confirm: ")
	answer, _ := reader.ReadString('\n')
	if strings.TrimSpace(answer) != "delete" {
		dim.Println("  Aborted.")
		return nil
	}

	fmt.Println()
	for _, t := range targets {
		img := rbd.GetImage(poolMap[t.pool], t.name)
		if err := img.Remove(); err != nil {
			red.Fprintf(os.Stderr, "  x  %s: %v\n", t.name, err)
		} else {
			green.Printf("  +  deleted %s\n", t.name)
		}
	}

	fmt.Println()
	return nil
}
