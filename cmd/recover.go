package cmd

import (
	"context"
	"fmt"
	"github.com/cleverhu/longhorn-backup-restore/pkg/k8s"
	"github.com/cleverhu/longhorn-backup-restore/pkg/util"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"strconv"
	"time"
)

func init() {
	log.SetLogger(zap.New())
}

func NewRecoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "recover",
		Long: "recover longhorn volumes and pvcs",
		RunE: recover,
	}

	return cmd
}

func recover(cmd *cobra.Command, args []string) error {
	if ApiEndpoint == "" {
		return fmt.Errorf("api-endpoint is empty")
	}
	httpClient := util.NewRequestClient()
	_, _, errors := httpClient.Get(ApiEndpoint).End()
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}

	logger := log.Log.WithName("longhorn-restore")

	c := k8s.GetClient(Kubeconfig)
	ctx := context.TODO()

	f, err := os.Open("./resources.yaml")
	cmdutil.CheckErr(err)
	decoder := yaml.NewYAMLOrJSONDecoder(f, 4096)

	for {
		unstruct := &unstructured.Unstructured{}
		err := decoder.Decode(unstruct)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
		logger.Info("restore object", "name", unstruct.GetName(), "namespace", unstruct.GetNamespace(), "kind", unstruct.GetKind())
		switch unstruct.GetKind() {
		case "PersistentVolumeClaim":
			pvc := &corev1.PersistentVolumeClaim{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.UnstructuredContent(), pvc)
			cmdutil.CheckErr(err)

			// wait for volume ready
			volume := &longhorn.Volume{}
		retry:
			err = c.Get(ctx, client.ObjectKey{Namespace: longhornNs, Name: pvc.Spec.VolumeName}, volume)
			if err != nil {
				if apierrors.IsNotFound(err) {
					time.Sleep(1 * time.Second)
					goto retry
				}
				cmdutil.CheckErr(err)
			}
			logger.Info("volume status", "name", volume.Name, "status", volume.Status.Robustness)
			if volume.Status.Robustness != longhorn.VolumeRobustnessHealthy {
				time.Sleep(1 * time.Second)
				goto retry
			}

			url := fmt.Sprintf("%s/v1/volumes/%s", ApiEndpoint, pvc.Spec.VolumeName)

			// create pv
			rsp, body, errors := httpClient.Post(url).Query("action=pvCreate").Type("json").Send(`{"pvName":"` + pvc.Spec.VolumeName + `","fsType":"ext4"`).End()
			if len(errors) > 0 {
				return utilerrors.NewAggregate(errors)
			}
			if rsp.StatusCode > 200 {
				return fmt.Errorf("restore pvc failed, response: %s", body)
			}
			logger.Info("restore pv", "response", body)
			// wait for pv ready

			// create pvc
			rsp, body, errors = httpClient.Post(url).Query("action=pvcCreate").Type("json").Send(`{"pvcName":"` + pvc.Name + `","namespace":"` + pvc.Namespace + `"}`).End()
			if len(errors) > 0 {
				return utilerrors.NewAggregate(errors)
			}
			if rsp.StatusCode > 200 {
				return fmt.Errorf("restore pvc failed, response: %s", body)
			}
			logger.Info("restore pvc", "response", body)
		case "Volume":
			volume := &longhorn.Volume{}
			size := unstruct.Object["spec"].(map[string]interface{})["size"].(string)
			sizeInt, _ := strconv.ParseInt(size, 10, 64)
			unstruct.Object["spec"].(map[string]interface{})["size"] = sizeInt
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.UnstructuredContent(), volume)
			cmdutil.CheckErr(err)
			err = c.Create(ctx, volume)
			if err != nil {
				if apierrors.IsAlreadyExists(err) {
					logger.Info("volume already exists, skip", "volume", volume.Name)
					continue
				}
				return err
			}
		}
	}
	logger.Info("restore finished")
	return nil
}
