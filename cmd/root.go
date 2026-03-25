package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	monitors   string
	cephUser   string
	keyfile    string
	key        string
	poolsFlag  string
	pools      []string
	kubeconfig string

	cyan   = color.New(color.FgCyan, color.Bold)
	green  = color.New(color.FgGreen, color.Bold)
	red    = color.New(color.FgRed, color.Bold)
	yellow = color.New(color.FgYellow, color.Bold)
	white  = color.New(color.FgWhite, color.Bold)
	dim    = color.New(color.FgHiBlack)
)

var rootCmd = &cobra.Command{
	Use:   "rbd-manager",
	Short: "Manage RBD volumes in the YetiMail Ceph cluster",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&monitors, "monitors", "", "Ceph monitor addresses, comma-separated (env: CEPH_MONITORS)")
	rootCmd.PersistentFlags().StringVar(&cephUser, "user", "", "CephX user (env: CEPH_USER, default: mailpoc_rbd)")
	rootCmd.PersistentFlags().StringVar(&keyfile, "keyfile", "", "Path to CephX keyring file (env: CEPH_KEYFILE)")
	rootCmd.PersistentFlags().StringVar(&key, "key", "", "CephX key string (env: CEPH_KEY)")
	rootCmd.PersistentFlags().StringVar(&poolsFlag, "pools", "", "RBD pools, comma-separated (env: CEPH_POOLS, default: both pools)")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (env: KUBECONFIG, default: ~/.kube/config)")
}

func validateConfig() error {
	resolveConfig()
	if len(pools) == 0 {
		return fmt.Errorf("no pools configured: set CEPH_POOLS or pass --pools")
	}
	return nil
}

func resolveConfig() {
	if monitors == "" {
		monitors = os.Getenv("CEPH_MONITORS")
	}
	if cephUser == "" {
		cephUser = os.Getenv("CEPH_USER")
	}
	if cephUser == "" {
		cephUser = "mailpoc_rbd"
	}
	if keyfile == "" {
		keyfile = os.Getenv("CEPH_KEYFILE")
	}
	if key == "" {
		key = os.Getenv("CEPH_KEY")
	}
	if poolsFlag == "" {
		poolsFlag = os.Getenv("CEPH_POOLS")
	}
	if poolsFlag != "" {
		pools = strings.Split(poolsFlag, ",")
		for i, p := range pools {
			pools[i] = strings.TrimSpace(p)
		}
	}
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
}
