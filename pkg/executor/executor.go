package executor

import (
	v1 "finupgroup.com/decision/traincrd/pkg/apis/v1"
	clientsetT "finupgroup.com/decision/traincrd/pkg/client/clientset/versioned"
	k8v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)


type Executor struct {
	clientTrain clientsetT.Interface
	clientK8s   kubernetes.Interface
}

func New(client clientsetT.Interface, clientK8 kubernetes.Interface) *Executor {
	return &Executor{clientTrain: client, clientK8s: clientK8}
}

func (exe *Executor) Run() {
	informer := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options k8v1.ListOptions) (object runtime.Object, e error) {
			return exe.clientTrain.DecisionV1().Traincrds(k8v1.NamespaceAll).List(options)
		},
		WatchFunc: func(options k8v1.ListOptions) (i watch.Interface, e error) {
			return exe.clientTrain.DecisionV1().Traincrds(k8v1.NamespaceAll).Watch(options)
		},
	},
		&v1.Traincrd{},
		0,
	)

	klog.Info("setup the handler for informer..")
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			train := obj.(*v1.Traincrd)
			klog.Infof("add train,  name: %s, ns: %s", train.Name, train.Namespace)
			traindeploy := traindeployBuild(train)
			traindeploy.clientK8s = exe.clientK8s

			err := traindeploy.trainCreate()
			if err != nil {
				klog.Fatal("创建 失败，", traindeploy.toString(), err.Error())
			} else {
				klog.Infof("创建 成功，", traindeploy.toString())
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			trainO := oldObj.(*v1.Traincrd)
			trainN := newObj.(*v1.Traincrd)
			traindeployO := traindeployBuild(trainO)
			traindeployN := traindeployBuild(trainN)

			// 部分可变属性发生变化时触发更新操作
			deployChanged := false
			if traindeployO.cpu != traindeployN.cpu || traindeployO.reqCpu != traindeployN.reqCpu ||  traindeployO.memory != traindeployN.memory || traindeployO.reqMemory != traindeployN.reqMemory ||  traindeployO.image != traindeployN.image{
				deployChanged = true
			}
			if !deployChanged {
				return
			}

			klog.Infof("update train, from %s to %s, \n", traindeployO.toString(), traindeployN.toString())
			traindeployO.clientK8s = exe.clientK8s
			_, err := traindeployO.updateOrGetDeployment(traindeployN)
			if err != nil {
				klog.Fatal("更新  失败，", traindeployO.toString(), err.Error())
			} else {
				klog.Infof("更新  成功，", traindeployO.toString())
			}
		},
		DeleteFunc: func(obj interface{}) {
			train := obj.(*v1.Traincrd)
			traindeploy := traindeployBuild(train)
			traindeploy.clientK8s = exe.clientK8s

			klog.Infof("delete train,  name: %s, ns: %s", train.Name, train.Namespace)
			err := traindeploy.deleteTrain()
			if err != nil {
				klog.Fatal("删除  失败，", traindeploy.toString(), err.Error())
			} else {
				klog.Infof("删除  成功，", traindeploy.toString())
			}
		},
	})

	stopCh := make(chan struct{})
	defer close(stopCh)

	informer.Run(stopCh)
}

func traindeployBuild(obj *v1.Traincrd) *Traindeploy {
	return &Traindeploy{
		name:      obj.Name,
		namespace: obj.Namespace,
		image:     obj.Spec.Image,
		username:  obj.Labels["username"],
		channel:   obj.Labels["channel"],
		cpu:       obj.Spec.Cpu,
		memory:    obj.Spec.Memory,
		reqCpu:    obj.Spec.ReqCpu,
		reqMemory: obj.Spec.ReqMemory,
		workDir:   "/user/" + obj.Name,
	}
}
