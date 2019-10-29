package pkg

import (
	"encoding/json"
	"fmt"
	"k8s.io/klog"
	"os"
	"testing"
)

type user struct {
	Name string `json:"name"`
	Addr string `json:"addr,omitempty"` //为空时忽略此 key
	Age int `json:"age,omitempty"` //为空时忽略此 key
}

func TestKlog(t *testing.T) {
	klog.SetOutput(os.Stdout)
	klog.InitFlags(nil)
	klog.Infof("hello %s .", "world")
}

func TestMarshal(t *testing.T) {
	u := user{Name: "31", Addr:"addr..."}
	data, _ := json.Marshal(u)
	fmt.Printf("%s\n", data)

	ou := user{}
	json.Unmarshal(data, &ou)
	fmt.Println(ou.Age)
}

func TestFmtString(t *testing.T) {
	str := fmt.Sprintf("/%s/", "usera")
	fmt.Println(str)
}