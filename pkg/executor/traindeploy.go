package executor

import (
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
const INGRESS_HOST_PROD = "mt.10.10.184.25.nip.io"

type Traindeploy struct {
	name      string
	username  string
	channel   string
	namespace string
	cpu       string
	memory    string
	reqCpu    string
	reqMemory string
	workDir   string
	image     string
	clientK8s kubernetes.Interface
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
	if !errors.IsNotFound(err) {
		return
	}

	klog.Infoln("删除 Service, ", t.name)
	err = t.deleteSvc()
	if !errors.IsNotFound(err) {
		return err
	}

	klog.Infoln("删除 Ingress, ", t.name)
	err = t.deleteIngress()
	if !errors.IsNotFound(err) {
		return err
	}

	klog.Infoln("删除 PVC, ", t.name)
	err = t.deletePVC()
	if !errors.IsNotFound(err) {
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
	createdDep, err := t.clientK8s.AppsV1().Deployments(t.namespace).Update(dep)

	return createdDep, err
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

	replicas := int32(1)
	deployLabels := map[string]string{"app": t.name, "username": t.username, "channel": t.channel}

	gracePeriodSeconds := int64(6 * 60)

	resources, err := getContainerResources(t)
	if err != nil {
		klog.Fatalln("生成 Resources 出现异常，", err)
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
							Ports: []corev1.ContainerPort{
								{
									Name:          "http-env",
									ContainerPort: int32(8888),
								},
							},
						},
					},
					ServiceAccountName:            "fission-fetcher",
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
						Host: INGRESS_HOST,
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
	storageQuantity, _ := resource.ParseQuantity("10Gi")

	existingPVC, err := t.clientK8s.CoreV1().PersistentVolumeClaims(t.namespace).Get(t.name, metav1.GetOptions{})
	if err == nil {
		return existingPVC, err
	}

	persistentVolumeClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: t.name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				// can be mounted in read/write mode to exactly 1 host
				corev1.ReadWriteOnce,
			},
			VolumeName:       t.name,
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
		" name:%s, username:%s, channel:s%, ns: %s, image:%s, cpu:%s, reqcpu:%s, mem:%s, reqmem:%s, ",
		t.name, t.username, t.channel, t.namespace, t.image, t.cpu, t.reqCpu, t.memory, t.reqMemory)
}
