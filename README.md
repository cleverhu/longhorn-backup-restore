# 说明
这个项目是为了完成 longhorn 跨集群迁移，以 wordpress 项目迁移为例。
longhorn 也支持数据备份，这项目完成一个集群的数据备份，还原到另外一个集群的功能。

# longhorn 安装说明
使用 longhorn 必须每一台节点安装 iscsi 工具，否则无法使用。
centos 安装方法:
```bash
yum install iscsi-initiator-utils -y
```
ubuntu 安装方法:
```bash
sudo apt-get install open-iscsi -y
```
## 额外说明
如果你想使用 longhorn ReadWriteMany 功能，你需要使用安装 nfs-client 工具，否则无法使用。
centos 安装方法:
```bash
yum install nfs-utils -y
```
ubuntu 安装方法:
```bash
sudo apt-get install nfs-common -y
```

## 部署 longhorn
修改 charts/longhorn/minio-secret.yaml，将地址修改为自己 s3 存储的。
修改 charts/longhorn/values.yaml，修改 s3 bucket backupTarget 为自己的 s3 bucket。
修改 defaultDataPath 为宿主机可用地址, 如果有需要可以加上 node selector。
修改后执行:
```bash
cd charts/longhorn
kubectl apply -f ./minio-secret.yaml
bash ./install.sh
```
先将 minio-secret.yaml 部署，再部署 longhorn，否则备份功能将无法使用。

## 部署 wordpress
可以修改 value.yaml 里面参数，例如设置登陆密码，mysql pvc 存储大小等参数。
修改后执行:
```bash
cd chatrs/wordpress
bash ./install.sh
```
# 迁移使用方法
在工作目录创建 bak-kubeconfig，写入需要备份的集群 kubeconfig。
在工作目录创建 recover-kubeconfig，写入需要还原的集群 kubeconfig。
运行`make bak` and `make recover` 即可完成备份和还原。
如果有必要，可以在还原之后，在还原集群执行 kubectl apply -f resources.yaml，这样会把 pvc 的 annos 和 labels 加回去。