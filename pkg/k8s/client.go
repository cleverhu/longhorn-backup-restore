package k8s

import (
	"sync"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Client client.Client
	o      sync.Once
	s      = scheme.Scheme
)

func init() {
	err := longhorn.AddToScheme(s)
	checkErr(err)
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func GetClient(kubeconfig string) client.Client {
	if Client == nil {
		o.Do(func() {
			cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			checkErr(err)

			c, err := client.New(cfg, client.Options{
				Scheme: s,
			})
			checkErr(err)
			Client = c
		})
	}

	return Client
}
