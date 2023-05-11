bak:
	go run main.go --kubeconfig=./bak-kubeconfig --api-endpoint=http://120.26.60.25:30001 bak

recover:
	go run main.go --kubeconfig=./recover-kubeconfig --api-endpoint=http://longhorn.study-k8s.com recover
