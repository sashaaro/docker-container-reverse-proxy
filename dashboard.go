package main

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"html/template"
	"log"
	"net/http"
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


	containers := map[string][]*types.Container{}

	for _, container := range ds.containerProxy.containers {
		projectName, ok := container.Labels["project.name"]
		if !ok {
			continue;
		}
		for _, networkSettings := range container.NetworkSettings.Networks {
			if network.ID == networkSettings.NetworkID {
				containers[projectName] = append(containers[projectName], container)
				break;
			}
		}
	}

	data := make(map[string]interface{})
	data["network"] = network
	data["containerProjects"] = containers
	data["selectedTargets"] = ds.containerProxy.selectedTargets

	t, _ := template.ParseFiles("network.html");
	t.Execute(w, data);
}

func (ds *Dashboard) handlePostTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		//w.WriteHeader(404);
		//w.Write([]byte("404"))
		//return
	}
	containerID := r.URL.Query().Get("container")
	port := r.URL.Query().Get("port")

	var container *types.Container;
	for _, cont := range ds.containerProxy.containers {
		if containerID == cont.ID {
			container = cont;
			break;
		}
	}

	if container == nil {
		w.WriteHeader(404);
		w.Write([]byte("404"))
		return
	}

	for _, selectedTarget := range ds.containerProxy.selectedTargets {
		if selectedTarget.port == port {
			selectedTarget.container = container;

			w.WriteHeader(200);
			return;
		}
	}

	w.WriteHeader(404);
	w.Write([]byte("404"));
}


func (ds *Dashboard) start() {
	http.HandleFunc("/network", ds.handleNetwork)
	http.HandleFunc("/target", ds.handlePostTarget)
	http.HandleFunc("/", ds.handlerMain)

	fmt.Println("Start dashboard on 8080 port")
	http.ListenAndServe(":8080", nil);
}

