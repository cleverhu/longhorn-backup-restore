package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	"github.com/cleverhu/longhorn-backup-restore/pkg/k8s"
	"github.com/cleverhu/longhorn-backup-restore/pkg/util"
)

const (
	longhornNs   = "longhorn-system"
	longhornSc   = "longhorn"
	resourceName = "resources.yaml"
)

func init() {
	log.SetLogger(zap.New())
}

type object struct {
	name      string
	namespace string
}

func NewBakCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "bak",
		Long: "backup longhorn volumes and pvcs",
		RunE: bak,
	}

	return cmd
}

func bak(cmd *cobra.Command, args []string) error {
	if ApiEndpoint == "" {
		return fmt.Errorf("api-endpoint is empty")
	}
	httpClient := util.NewRequestClient()
	_, _, errors := httpClient.Get(ApiEndpoint).End()
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}

	logger := log.Log.WithName("longhorn-backup")

	c := k8s.GetClient(Kubeconfig)
	ctx := context.TODO()

	// 列举 longhorn volumes
	volumeList := &longhorn.VolumeList{}
	err := c.List(ctx, volumeList, client.InNamespace(longhornNs))
	cmdutil.CheckErr(err)

	// 给所有 longhorn volumes 打快照, 命名空间为 longhorn-system
	volumesMap := map[string]longhorn.Volume{}
	createdSps := make([]object, 0, len(volumeList.Items))
	for _, volume := range volumeList.Items {
		logger.Info("volume", volume.Name, "namespace", volume.Namespace)

		url := fmt.Sprintf("%s/v1/volumes/%s", ApiEndpoint, volume.Name)
		rsp, _, errors := httpClient.Post(url).Query(`action=snapshotCreate`).Type("json").Send(`{"test": "test"}`).End()
		if len(errors) > 0 {
			logger.Error(errors[0], "create snapshot failed")
			return utilerrors.NewAggregate(errors)
		}

		sp := map[string]interface{}{}
		err = json.NewDecoder(rsp.Body).Decode(&sp)
		cmdutil.CheckErr(err)

		logger.Info("created snapshot", "snapshot", sp)
		createdSps = append(createdSps, object{
			name:      sp["name"].(string),
			namespace: longhornNs,
		})
		volumesMap[volume.Name] = volume
		rsp.Body.Close()
	}

	// 创建备份，获取备份地址
	createdBackups := make([]object, 0, len(volumeList.Items))
	for i := 0; i < len(volumeList.Items); i++ {
		sp := createdSps[i]
		snapshot := &longhorn.Snapshot{}
		err = c.Get(ctx, client.ObjectKey{
			Namespace: sp.namespace,
			Name:      sp.name,
		}, snapshot)

		if err != nil {
			logger.Error(err, "get snapshot error", "name", sp.name, "namespace", sp.namespace)
			if apierrors.IsNotFound(err) {
				time.Sleep(time.Second)
				i--
				continue
			}
			cmdutil.CheckErr(err)
		}

		url := fmt.Sprintf("%s/v1/volumes/%s", ApiEndpoint, snapshot.Spec.Volume)
		rsp, _, errors := httpClient.Post(url).Query(`action=snapshotBackup`).Type("json").Send(`{"name":"` + sp.name + `"}`).End()
		if len(errors) > 0 {
			logger.Error(errors[0], "create snapshot failed")
			return utilerrors.NewAggregate(errors)
		}

		backup := map[string]interface{}{}
		err = json.NewDecoder(rsp.Body).Decode(&backup)
		cmdutil.CheckErr(err)

		backupStatus := backup["backupStatus"].([]interface{})[0]
		createdBackups = append(createdBackups, object{
			name:      backupStatus.(map[string]interface{})["id"].(string),
			namespace: longhornNs,
		})
		logger.Info("create backup", "backup", backup)
	}

	// 等待所有的备份完成
	// 修改所有 volumes 的 spec.nodeID 为空, 设置 volumes spec.fromBackup 为备份地址
	// 等待快照完成
backuploop:
	for _, bu := range createdBackups {
		backup := &longhorn.Backup{}
		err = c.Get(ctx, client.ObjectKey{
			Namespace: bu.namespace,
			Name:      bu.name,
		}, backup)
		if err != nil {
			logger.Error(err, "get backup error", "name", bu.name, "namespace", bu.namespace)
			goto backuploop
		}

		logger.Info("backup", "name", backup.Name, "state", backup.Status.State)
		if backup.Status.State != longhorn.BackupStateCompleted {
			time.Sleep(5 * time.Second)
			goto backuploop
		}
		volume := volumesMap[backup.Status.VolumeName]
		volume.Spec.NodeID = ""
		volume.Spec.FromBackup = backup.Status.URL
		volumesMap[backup.Status.VolumeName] = volume
	}

	logger.Info("all backups are ready")

	f, err := os.OpenFile("./resources.yaml", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	cmdutil.CheckErr(err)

	defer f.Close()
	// 将所有 volume 输出到一个 yaml 文件, volumes.yaml
	for uid := range volumesMap {
		volume := volumesMap[uid]
		logger.Info("volume", "name", volume.Name, "namespace", volume.Namespace)
		err = writeObject(&volume, f)
		cmdutil.CheckErr(err)
	}
	logger.Info("backup volumes successfully")

	// 将 所有 longhorn pvc 输出到一个 yaml 文件, pvc.yaml
	pvcList := &corev1.PersistentVolumeClaimList{}
	err = c.List(ctx, pvcList)
	cmdutil.CheckErr(err)

	for _, pvc := range pvcList.Items {
		pvc.APIVersion = "v1"
		pvc.Kind = "PersistentVolumeClaim"
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName == longhornSc {
			logger.Info("pvc", "name", pvc.Name, "namespace", pvc.Namespace)
			err = writeObject(&pvc, f)
			if err != nil {
				return err
			}
		}
	}
	logger.Info("backup pvcs successfully")
	logger.Info("backup successfully")
	return nil
}

func writeObject(o metav1.Object, writer *os.File) error {
	o.SetManagedFields(nil)
	o.SetResourceVersion("")
	b, _ := json.Marshal(o)
	bytes, _ := yaml.JSONToYAML(b)
	_, err := writer.Write(bytes)
	if err != nil {
		return err
	}

	_, err = writer.WriteString("---\n")
	return err
}
