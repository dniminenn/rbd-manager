package cmd

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/spf13/cobra"
)

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "List RBD images with no Bound PV (orphans and Released)",
	RunE:  runOrphans,
}

func init() {
	rootCmd.AddCommand(orphansCmd)
}

func runOrphans(cmd *cobra.Command, args []string) error {
	conn, err := connectCeph()
	if err != nil {
		return err
	}
	defer conn.Shutdown()

	cs, err := connectK8s()
	if err != nil {
		return err
	}

	pvImages, err := rbdImagesFromPVs(cs)
	if err != nil {
		return err
	}

	images, err := listAllImages(conn)
	if err != nil {
		return err
	}

	type candidate struct {
		img    imageInfo
		status string
		pvName string
	}
	var candidates []candidate

	for _, img := range images {
		if pv, ok := pvImages[img.Name]; ok {
			if pv.Phase == corev1.VolumeBound {
				continue
			}
			candidates = append(candidates, candidate{
				img:    img,
				status: string(pv.Phase),
				pvName: pv.PVName,
			})
		} else {
			candidates = append(candidates, candidate{img: img, status: "ORPHAN"})
		}
	}

	fmt.Println()
	cyan.Println("  Reclaimable RBD images")
	fmt.Println()

	if len(candidates) == 0 {
		green.Println("  Nothing to reclaim. All images have a Bound PV.")
		fmt.Println()
		return nil
	}

	fmt.Printf("  %s  %s  %s  %s  %s\n",
		dim.Sprint(col(24, "POOL")),
		dim.Sprint(col(44, "IMAGE")),
		dim.Sprint(col(9, "SIZE")),
		dim.Sprint(col(9, "STATUS")),
		dim.Sprint("PV"))
	dim.Println("  " + strings.Repeat("─", 100))

	var totalSize uint64
	for _, c := range candidates {
		totalSize += c.img.Size
		var status string
		if c.status == "ORPHAN" {
			status = red.Sprint(col(9, "ORPHAN"))
		} else {
			status = yellow.Sprint(col(9, c.status))
		}
		fmt.Printf("  %s  %s  %s  %s  %s\n",
			dim.Sprint(col(24, c.img.Pool)),
			white.Sprint(col(44, c.img.Name)),
			col(9, humanSize(c.img.Size)),
			status,
			dim.Sprint(c.pvName))
	}

	fmt.Println()
	dim.Println("  " + strings.Repeat("─", 100))
	fmt.Printf("  %s image(s) reclaimable, %s total\n",
		yellow.Sprintf("%d", len(candidates)), humanSize(totalSize))
	fmt.Println()
	dim.Println("  To delete:  rbd-manager delete <image-name> [...]")
	fmt.Println()

	return nil
}
