package main

import (
	clientsetTrain "finupgroup.com/decision/traincrd/pkg/client/clientset/versioned"
	"finupgroup.com/decision/traincrd/pkg/executor"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"os"
	"os/signal"
	"syscall"
)


func main() {

	klog.SetOutput(os.Stdout)
	klog.InitFlags(nil)

	clientT, clientK8s, err := getk8sclient()

	if err != nil {
		klog.Fatalf("Error building example clientset: %v", err)
	}


	klog.Info("run executor with client")
	exe := executor.New(clientT, clientK8s)
	go exe.Run()


	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}

func getk8sclient() (clientsetTrain.Interface, clientset.Interface, error){
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, err
	}

	// creates the clientset
	clientsetT, err := clientsetTrain.NewForConfig(config)
	clientsetK8, err := clientset.NewForConfig(config)

	if err != nil {
		return nil,nil,  err
	}

	return clientsetT,clientsetK8, nil
}