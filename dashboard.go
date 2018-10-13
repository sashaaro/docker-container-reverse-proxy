package main

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"html/template"
	"log"
	"net/http"
	"strings"
)

type Dashboard struct {
	containerProxy *ContainerProxy
}


func (ds *Dashboard) handlerMain(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("main.html");
	if err != nil {
		log.Print(err)
		return
	}
	fmt.Println("containers %s", len(ds.containerProxy.containers))
	join := Map(ds.containerProxy.networks, func(n *types.NetworkResource) string {
		return n.Name;
	})
	fmt.Println("networks %s", strings.Join(join, " "))

	data := make(map[string]interface{})
	data["containers"] = ds.containerProxy.containers
	data["networks"] = ds.containerProxy.networks
	t.Execute(w, data);
}

func (ds *Dashboard) handleNetwork(w http.ResponseWriter, r *http.Request) {
	networkID := r.URL.Query().Get("id")

	var network *types.NetworkResource;
	for _, net := range ds.containerProxy.networks {
		if networkID == net.ID {
			network = net;
			break;
		}
	}

	if network == nil {
		w.WriteHeader(404);
		w.Write([]byte("404"))
		return
	}

	data := make(map[string]interface{})
	data["network"] = network
	t, _ := template.ParseFiles("network.html");
	t.Execute(w, data);
}

func (ds *Dashboard) start() {
	http.HandleFunc("/network", ds.handleNetwork)
	http.HandleFunc("/", ds.handlerMain)

	fmt.Println(fmt.Sprintf("Start dashboard on 8080 port"))
	http.ListenAndServe(":8080", nil);
}

