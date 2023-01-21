package integration

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// For client auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const APP_NAME = "MetalLB"

var kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
var namespace = flag.String("namespace", "", "namespace name in which the application is deployed")

var ipAddressPoolType = schema.GroupVersionResource{Group: "metallb.io", Version: "v1beta1", Resource: "ipaddresspools"}
var l2AdvertisementType = schema.GroupVersionResource{Group: "metallb.io", Version: "v1beta1", Resource: "l2advertisements"}

type IpAddressPoolConfig struct {
	name        string
	addressFrom string
	addressTo   string
}
type L2AdvertisementConfig struct {
	name              string
	ipAddressPoolName string
}

func clusterConfigOrDie() *rest.Config {
	var config *rest.Config
	var err error

	if *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		panic(err.Error())
	}

	return config
}

func createServiceOrDie(ctx context.Context, tC corev1.CoreV1Interface, name string) *v1.Service {
	servicePayload := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: *namespace,
			Name:      name,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name: "vib-tests-example",
					Port: 8080,
					TargetPort: intstr.IntOrString{
						IntVal: 8081,
					},
				},
			},
			Type: v1.ServiceTypeLoadBalancer,
		},
	}

	res, err := tC.Services(*namespace).Create(ctx, servicePayload, metav1.CreateOptions{})
	if err != nil {
		panic(fmt.Sprintf("There was an error creating the Service: %s", err))
	}

	return res
}

func createIpAddressPoolResourceOrDie(ctx context.Context, dC dynamic.Interface, config *IpAddressPoolConfig) *unstructured.Unstructured {
	payload := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "metallb.io/v1beta1",
			"kind":       "IPAddressPool",
			"metadata": map[string]interface{}{
				"name":      config.name,
				"namespace": *namespace,
			},
			"spec": map[string]interface{}{
				"addresses": []string{
					fmt.Sprintf("%s-%s", config.addressFrom, config.addressTo),
				},
			},
		},
	}

	res, err := dC.Resource(ipAddressPoolType).Namespace(*namespace).Create(ctx, payload, metav1.CreateOptions{})
	if err != nil {
		panic(fmt.Sprintf("There was an error creating the IpAddressPool: %s", err))
	}

	return res
}

func createL2AdvertisementResourceOrDie(ctx context.Context, dC dynamic.Interface, config *L2AdvertisementConfig) *unstructured.Unstructured {
	payload := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "metallb.io/v1beta1",
			"kind":       "L2Advertisement",
			"metadata": map[string]interface{}{
				"name":      config.name,
				"namespace": *namespace,
			},
			"spec": map[string]interface{}{
				"ipAddressPools": []string{
					config.ipAddressPoolName,
				},
			},
		},
	}

	res, err := dC.Resource(l2AdvertisementType).Namespace(*namespace).Create(ctx, payload, metav1.CreateOptions{})
	if err != nil {
		panic(fmt.Sprintf("There was an error creating the L2AdvertisementResource: %s", err))
	}

	return res
}

func checkValidIP(ipToCheck string, addressFrom string, addressTo string) bool {
	parsedIp := net.ParseIP(ipToCheck)
	ipFrom := net.ParseIP(addressFrom)
	ipTo := net.ParseIP(addressTo)

	if ipFrom == nil || ipTo == nil {
		panic("Valid IP ranges should be provided")
	}

	if parsedIp != nil {
		if parsedIp.To4() != nil {
			if bytes.Compare(parsedIp, ipFrom) >= 0 && bytes.Compare(parsedIp, ipTo) <= 0 {
				return true
			}
		}
	}
	return false
}

func CheckRequirements() {
	if *namespace == "" {
		panic(fmt.Sprintf("The namespace where %s is deployed must be provided. Use the '--namespace' flag", APP_NAME))
	}
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	CheckRequirements()
	RunSpecs(t, fmt.Sprintf("%s Integration Tests", APP_NAME))
}
