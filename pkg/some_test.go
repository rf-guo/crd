package pkg

import (
	"k8s.io/klog"
	"os"
	"testing"
)

func TestKlog(t *testing.T) {
	klog.SetOutput(os.Stdout)
	klog.InitFlags(nil)
	klog.Infof("hello %s .", "world")
}