package cmd

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type pvInfo struct {
	PVName    string
	PVCName   string
	Namespace string
	Pool      string
	DataPool  string // non-empty for EC-backed volumes
	Phase     corev1.PersistentVolumePhase
}

func connectK8s() (*kubernetes.Clientset, error) {
	resolveConfig()

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}
	return cs, nil
}

// rbdImagesFromPVs returns a map of RBD image name to PV info for all PVs
// backed by rbd.csi.ceph.com in any of the configured pools.
func rbdImagesFromPVs(cs *kubernetes.Clientset) (map[string]pvInfo, error) {
	pvList, err := cs.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list PVs: %w", err)
	}

	poolSet := make(map[string]bool, len(pools))
	for _, p := range pools {
		poolSet[p] = true
	}

	result := make(map[string]pvInfo)
	for _, pv := range pvList.Items {
		if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != "rbd.csi.ceph.com" {
			continue
		}
		attrs := pv.Spec.CSI.VolumeAttributes
		if attrs == nil || attrs["imageName"] == "" {
			continue
		}
		pvPool := attrs["pool"]
		if !poolSet[pvPool] {
			continue
		}

		info := pvInfo{
			PVName:   pv.Name,
			Phase:    pv.Status.Phase,
			Pool:     pvPool,
			DataPool: attrs["dataPool"],
		}
		if pv.Spec.ClaimRef != nil {
			info.PVCName = pv.Spec.ClaimRef.Name
			info.Namespace = pv.Spec.ClaimRef.Namespace
		}
		result[attrs["imageName"]] = info
	}
	return result, nil
}
