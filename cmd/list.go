package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var filterFlag string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all RBD images with PV binding status",
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringVarP(&filterFlag, "filter", "f", "", "Filter rows by substring (matches image name, pool, PV, or PVC)")
	rootCmd.AddCommand(listCmd)
}

func col(width int, s string) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func runList(cmd *cobra.Command, args []string) error {
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

	fmt.Println()
	if filterFlag != "" {
		cyan.Printf("  RBD images  (filter: %s)\n", filterFlag)
	} else {
		cyan.Println("  RBD images")
	}
	fmt.Println()
	fmt.Printf("  %s  %s  %s  %s  %s  %s  %s\n",
		dim.Sprint(col(24, "POOL")),
		dim.Sprint(col(44, "IMAGE")),
		dim.Sprint(col(9, "SIZE")),
		dim.Sprint(col(9, "STATUS")),
		dim.Sprint(col(7, "EC DATA")),
		dim.Sprint(col(44, "PV")),
		dim.Sprint("PVC"))
	dim.Println("  " + strings.Repeat("─", 152))

	var totalSize uint64
	var boundCount, releasedCount, orphanCount, shown int

	for _, img := range images {
		var statusStr, pvName, claim, dataPool string

		if img.Err != nil {
			statusStr = "error"
			claim = img.Err.Error()
		} else if pv, ok := pvImages[img.Name]; ok {
			statusStr = string(pv.Phase)
			pvName = pv.PVName
			dataPool = pv.DataPool
			if pv.PVCName != "" {
				claim = pv.Namespace + "/" + pv.PVCName
			} else {
				claim = "-"
			}
		} else {
			statusStr = "ORPHAN"
		}

		if filterFlag != "" {
			needle := strings.ToLower(filterFlag)
			if !strings.Contains(strings.ToLower(img.Name), needle) &&
				!strings.Contains(strings.ToLower(img.Pool), needle) &&
				!strings.Contains(strings.ToLower(dataPool), needle) &&
				!strings.Contains(strings.ToLower(pvName), needle) &&
				!strings.Contains(strings.ToLower(claim), needle) {
				continue
			}
		}

		shown++
		totalSize += img.Size

		var colorStatus string
		switch statusStr {
		case "Bound":
			colorStatus = green.Sprint(col(9, statusStr))
			boundCount++
		case "Released":
			colorStatus = yellow.Sprint(col(9, statusStr))
			releasedCount++
		case "ORPHAN":
			colorStatus = red.Sprint(col(9, statusStr))
			orphanCount++
		default:
			colorStatus = red.Sprint(col(9, statusStr))
		}

		ecTag := col(7, "")
		if dataPool != "" {
			ecTag = yellow.Sprint(col(7, "[EC]"))
		}
		fmt.Printf("  %s  %s  %s  %s  %s  %s  %s\n",
			dim.Sprint(col(24, img.Pool)),
			white.Sprint(col(44, img.Name)),
			col(9, humanSize(img.Size)),
			colorStatus,
			ecTag,
			dim.Sprint(col(44, pvName)),
			dim.Sprint(claim))
	}

	fmt.Println()
	dim.Println("  " + strings.Repeat("─", 145))
	if filterFlag != "" {
		fmt.Printf("  %d shown  |  %s bound  %s released  %s orphaned  |  %s total\n",
			shown,
			green.Sprintf("%d", boundCount),
			yellow.Sprintf("%d", releasedCount),
			red.Sprintf("%d", orphanCount),
			humanSize(totalSize))
	} else {
		fmt.Printf("  %d image(s), %s total  |  %s bound  %s released  %s orphaned\n",
			len(images), humanSize(totalSize),
			green.Sprintf("%d", boundCount),
			yellow.Sprintf("%d", releasedCount),
			red.Sprintf("%d", orphanCount))
	}
	fmt.Println()

	return nil
}
