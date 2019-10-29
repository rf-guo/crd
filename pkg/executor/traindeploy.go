package executor

import (
	v1 "finupgroup.com/decision/traincrd/pkg/apis/v1"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

const INGRESS_HOST = "mt.10.10.184.25.nip.io"
const INGRESS_HOST_PROD = "train-lab.finupgroup.com"
const PUBLIC_STORAGE = "trainlabpublicstorage"
const PUBLIC_LIBS_STORAGE = "trainlabpublic-libs-storage"
const PUBLIC_LIBS_VOLUME = "/usr/crd/lib/"

type Traindeploy struct {
	name      string
	username  string
	channel   string
	namespace string
	cpu       string
	memory    string
	reqCpu    string
	reqMemory string
	replicas  int
	workDir   string
	image     string
	capacity  string
	clientK8s kubernetes.Interface
}

/**
通过 CRD  类型 Traincrd 构建 traindeploy 配置
*/
func traindeployBuild(obj *v1.Traincrd) *Traindeploy {
	t := &Traindeploy{
		name:      obj.Name,
		namespace: obj.Namespace,
		image:     obj.Spec.Image,
		username:  obj.Labels["username"],
		channel:   obj.Labels["channel"],
		cpu:       obj.Spec.Cpu,
		memory:    obj.Spec.Memory,
		reqCpu:    obj.Spec.ReqCpu,
		reqMemory: obj.Spec.ReqMemory,
		replicas:  obj.Spec.Replicas,
		capacity:  obj.Spec.Capacity,
	}
	t.workDir = fmt.Sprintf("/%s/%s/%s/", t.channel, t.username, t.name)

	return t
}

func (t *Traindeploy) trainCreate() error {
	klog.Infoln("创建 Deployment, ", t.name)
	_, err := t.createOrGetDeployment()
	if err != nil {
		return err
	}

	klog.Infoln("创建 Service, ", t.name)
	_, err = t.createOrGetSvc()
	if err != nil {
		return err
	}

	klog.Infoln("创建 Ingress, ", t.name)
	_, err = t.createOrGetIngress()
	if err != nil {
		return err
	}

	klog.Infoln("创建 PVC, ", t.name)
	_, err = t.createOrGetPersistentVolumeClaim()
	if err != nil {
		return err
	}

	return err
}

func (t *Traindeploy) deleteTrain() (err error) {
	klog.Infoln("删除 Deployment, ", t.name)
	err = t.deleteDeployment()
	if errors.IsNotFound(err) {
		return
	}

	klog.Infoln("删除 Service, ", t.name)
	err = t.deleteSvc()
	if errors.IsNotFound(err) {
		return err
	}

	klog.Infoln("删除 Ingress, ", t.name)
	err = t.deleteIngress()
	if errors.IsNotFound(err) {
		return err
	}

	klog.Infoln("删除 PVC, ", t.name)
	err = t.deletePVC()
	if errors.IsNotFound(err) {
		return err
	}

	return
}

/**
Traincrd Deployment CRUDs
*/

func (t *Traindeploy) createOrGetDeployment() (*appsv1.Deployment, error) {

	existingDep, err := t.clientK8s.AppsV1().Deployments(t.namespace).Get(t.name, metav1.GetOptions{})

	//err is nil, exist
	if err == nil {
		klog.Infof("添加时发现已存在 deployment 不做任何操作，可能是restart 后 reload， ", t.toString())
		return existingDep, nil
	}

	// not exist, create !
	if errors.IsNotFound(err) {
		dep, err := t.makeDeploymentSpec()
		if err != nil {
			return nil, err
		}
		createdDep, newErr := t.clientK8s.AppsV1().Deployments(t.namespace).Create(dep)

		return createdDep, newErr
	}

	return nil, err
}

func (t *Traindeploy) updateOrGetDeployment(newTrain *Traindeploy) (*appsv1.Deployment, error) {

	_, err := t.clientK8s.AppsV1().Deployments(t.namespace).Get(t.name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return newTrain.createOrGetDeployment()
	}
	if err != nil {
		return nil, err
	}

	dep, err := newTrain.makeDeploymentSpec()
	if err != nil {
		return nil, err
	}
	updatedDep, err := t.clientK8s.AppsV1().Deployments(t.namespace).Update(dep)

	return updatedDep, err
}

func (t *Traindeploy) deleteDeployment() error {
	_, err := t.clientK8s.AppsV1().Deployments(t.namespace).Get(t.name, metav1.GetOptions{})
	if err != nil {
		return err
	} else {
		err := t.clientK8s.AppsV1().Deployments(t.namespace).Delete(t.name, &metav1.DeleteOptions{})
		return err
	}
}

func (t *Traindeploy) makeDeploymentSpec() (*appsv1.Deployment, error) {

	deployLabels := map[string]string{"app": t.name, "username": t.username, "channel": t.channel}

	gracePeriodSeconds := int64(1 * 60) //优雅关闭等待时长
	replicas := int32(t.replicas)

	resources, err := getContainerResources(t)
	if err != nil {
		klog.Errorln("生成 Resources 出现异常，", err)
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   t.name,
			Labels: deployLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: deployLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: deployLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            t.name,
							Image:           t.image,
							ImagePullPolicy: corev1.PullAlways,
							Resources:       resources,
							Env: []corev1.EnvVar{
								{Name: "NAME", Value: t.name},
								{Name: "BASE_DIR", Value: t.name},
								{Name: "WORK_DIR", Value: t.workDir},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      t.name,
									MountPath: fmt.Sprintf("/%s/%s/%s/", t.channel, t.username, t.name),
								},
								{
									Name:      PUBLIC_STORAGE,
									MountPath: "/public",
								},
								{
									Name:      PUBLIC_LIBS_STORAGE,
									MountPath: PUBLIC_LIBS_VOLUME,
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-env",
									ContainerPort: int32(8888),
								},
							},
						},
					},
					ServiceAccountName: "fission-svc",
					Volumes: []corev1.Volume{
						{
							Name: t.name,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: t.name,
								},
							},
						},
						{
							Name: PUBLIC_STORAGE,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: PUBLIC_STORAGE,
								},
							},
						},
						{
							Name: PUBLIC_LIBS_STORAGE,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: PUBLIC_LIBS_STORAGE,
								},
							},
						},
					},
					TerminationGracePeriodSeconds: &gracePeriodSeconds,
				},
			},
		},
	}

	return deployment, nil
}

func getContainerResources(t *Traindeploy) (corev1.ResourceRequirements, error) {
	mincpu, err := resource.ParseQuantity(t.reqCpu)
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}

	minmem, err := resource.ParseQuantity(t.reqMemory)
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}

	maxcpu, err := resource.ParseQuantity(t.cpu)
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}

	maxmem, err := resource.ParseQuantity(t.memory)
	if err != nil {
		return corev1.ResourceRequirements{}, err
	}

	return corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    mincpu,
			corev1.ResourceMemory: minmem,
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    maxcpu,
			corev1.ResourceMemory: maxmem,
		},
	}, nil
}

/**
Traincrd Service CRUDs
*/

func (t *Traindeploy) createOrGetSvc() (*corev1.Service, error) {
	existingSvc, err := t.clientK8s.CoreV1().Services(t.namespace).Get(t.name, metav1.GetOptions{})
	deployLabels := map[string]string{"app": t.name, "username": t.username, "channel": t.channel}

	if err == nil {
		return existingSvc, err
	} else if errors.IsNotFound(err) {
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:   t.name,
				Labels: deployLabels,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:       "http",
						Port:       8888,
						TargetPort: intstr.FromInt(8888),
					},
				},
				Selector: deployLabels,
				Type:     corev1.ServiceTypeClusterIP,
			},
		}

		svc, err := t.clientK8s.CoreV1().Services(t.namespace).Create(service)
		if err != nil {
			return nil, err
		}
		return svc, nil
	}

	return nil, err
}

func (t *Traindeploy) deleteSvc() error {

	_, err := t.clientK8s.CoreV1().Services(t.namespace).Get(t.name, metav1.GetOptions{})
	if err != nil {
		return err
	} else {
		err := t.clientK8s.CoreV1().Services(t.namespace).Delete(t.name, &metav1.DeleteOptions{})
		return err
	}

}

/**
Traincrd Ingress CRUDs
*/

func (t *Traindeploy) createOrGetIngress() (*v1beta1.Ingress, error) {
	existingIngs, err := t.clientK8s.ExtensionsV1beta1().Ingresses(t.namespace).Get(t.name, metav1.GetOptions{})
	labels := map[string]string{}
	if err == nil {
		return existingIngs, err
	} else if errors.IsNotFound(err) {
		ingress := &v1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:   t.name,
				Labels: labels,
			},
			Spec: v1beta1.IngressSpec{
				Rules: []v1beta1.IngressRule{
					{
						Host: INGRESS_HOST_PROD,
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									{
										Path: "/" + t.name,
										Backend: v1beta1.IngressBackend{
											ServiceName: t.name,
											ServicePort: intstr.IntOrString{
												Type:   intstr.Int,
												IntVal: 8888,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		ings, err := t.clientK8s.ExtensionsV1beta1().Ingresses(t.namespace).Create(ingress)
		return ings, err
	}
	return nil, nil
}

func (t *Traindeploy) deleteIngress() error {
	_, err := t.clientK8s.ExtensionsV1beta1().Ingresses(t.namespace).Get(t.name, metav1.GetOptions{})
	if err != nil {
		return err
	} else {
		err := t.clientK8s.ExtensionsV1beta1().Ingresses(t.namespace).Delete(t.name, &metav1.DeleteOptions{})
		return err
	}
}

/**
PVC  CRUDs
*/
func (t *Traindeploy) createOrGetPersistentVolumeClaim() (*corev1.PersistentVolumeClaim, error) {
	storageClassName := "cephfs"
	capacity := "1Gi"
	if t.capacity != "" {
		capacity = t.capacity
	}
	storageQuantity, _ := resource.ParseQuantity(capacity)

	existingPVC, err := t.clientK8s.CoreV1().PersistentVolumeClaims(t.namespace).Get(t.name, metav1.GetOptions{})
	if err == nil {
		return existingPVC, err
	}

	pvcAnn := map[string]string{
		"volume.beta.kubernetes.io/storage-class":       "cephfs",
		"volume.beta.kubernetes.io/storage-provisioner": "ceph.com/cephfs",
	}
	persistentVolumeClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        t.name,
			Annotations: pvcAnn,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				// can be mounted in read/write mode to exactly 1 host
				corev1.ReadWriteMany,
			},
			StorageClassName: &storageClassName,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageQuantity,
				},
			},
		},
	}
	pvc, err := t.clientK8s.CoreV1().PersistentVolumeClaims(t.namespace).Create(persistentVolumeClaim)
	return pvc, err
}

func (t *Traindeploy) deletePVC() (err error) {
	_, err = t.clientK8s.CoreV1().PersistentVolumeClaims(t.namespace).Get(t.name, metav1.GetOptions{})
	if err != nil {
		return
	}

	err = t.clientK8s.CoreV1().PersistentVolumeClaims(t.namespace).Delete(t.name, &metav1.DeleteOptions{})
	return
}

func (t *Traindeploy) toString() string {
	return fmt.Sprintf(
		" name:%s, username:%s, channel:s%, ns: %s, image:%s, cpu:%s, reqcpu:%s, mem:%s, reqmem:%s, replicas:%s, ",
		t.name, t.username, t.channel, t.namespace, t.image, t.cpu, t.reqCpu, t.memory, t.reqMemory, t.replicas)
}
