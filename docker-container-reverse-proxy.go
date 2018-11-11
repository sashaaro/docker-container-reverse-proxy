package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/google/tcpproxy"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

type ContainerProxy struct {
	networkPattern string
	containers []*types.Container
	networks []*types.NetworkResource
	selectedTargets []*SelectedContainerTarget
	cli *client.Client
}

func (this *ContainerProxy) createClient () {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))

	if err != nil {
		panic(err)
		return;
	}
	this.cli = cli

}

func (this *ContainerProxy) loadContainers () {
	fmt.Println("Updating containers info")
	this.containers = []*types.Container{}
	this.networks = []*types.NetworkResource{}

	containers, err := this.cli.ContainerList(context.Background(), types.ContainerListOptions{})

	if err != nil {
		fmt.Printf(err.Error());
		return;
	}

	networks, err := this.cli.NetworkList(context.Background(), types.NetworkListOptions{})

	if err != nil {
		fmt.Printf(err.Error());
		return;
	}

	for _, network := range networks {
		matched, err := regexp.MatchString(this.networkPattern, network.Name)
		if err != nil {
			fmt.Printf(err.Error());
			return;
		}

		if (matched) {
			networkRef := network
			this.networks = append(this.networks, &networkRef);
		}
	}

	// get aliases and fill
	for _, container := range containers {
		for _, filteredNetwork := range this.networks {
			if containerNetwork, ok := container.NetworkSettings.Networks[filteredNetwork.Name]; ok {
				con, _ := this.cli.ContainerInspect(context.Background(), container.ID)
				containerNetwork.Aliases = con.NetworkSettings.Networks[filteredNetwork.Name].Aliases

				containerRef := container
				this.containers = append(this.containers, &containerRef);
				continue;
			}
		}
	}
}

func debounce(events <-chan events.Message, d time.Duration, callback func()) {
	var timer *time.Timer
	for _ = range events {
		go func() {
			if timer == nil {
				timer = time.NewTimer(d)
				<-timer.C
				timer.Stop()
				timer = nil
				go callback()
			}
		}()
	}
}

func (this ContainerProxy) listen () {
	events, _ := this.cli.Events(context.Background(), types.EventsOptions{})
	debounce(events, 10 * time.Second, func() {
		this.loadContainers()
	})
}

type ContainerByAliasesTarget struct {
	containerProxy *ContainerProxy
	targetPort string
}

func (ch *ContainerByAliasesTarget) HandleConn(conn net.Conn)  {
	wrap, ok := conn.(*tcpproxy.Conn);
	if !ok {
		conn.Close()
		return
	}
	if wrap.HostName == "" {
		conn.Close()
		return;
	}

	for _, container := range ch.containerProxy.containers {
		for _, network := range container.NetworkSettings.Networks {
			for _, alias := range network.Aliases {
				if strings.Index(wrap.HostName, alias) == 0 { //with any port
					hostAddress := getHostAddress(container)
					if hostAddress == "" {
						// log wtf
						return
					}
					addr := fmt.Sprintf("%s:%s", hostAddress, ch.targetPort)
					fmt.Printf("Forwarding %s to %s\n", wrap.HostName, addr)
					dp := &tcpproxy.DialProxy{Addr: addr}
					dp.HandleConn(conn)
					return
				}
			}
		}
	}

	conn.Close()
}

func getHostAddress(container *types.Container) string {
	hNetwork, ok := container.NetworkSettings.Networks[container.HostConfig.NetworkMode]
	if !ok {
		// return nil, fmt.Errorf("unable to find network settings for the network %s", networkMode)
		return "";
	}
	return hNetwork.IPAddress;
}

func withHttpHostPattern(httpHostPattern string) tcpproxy.Matcher {
	return func(_ context.Context, got string) bool {
		matched, err := regexp.MatchString(httpHostPattern, got)
		if err != nil {
			panic(err)
		}
		return matched
	}
}

type SelectedContainerTarget struct {
	Name string
	Port string
	Container *types.Container
}

func (tagret *SelectedContainerTarget) HandleConn(conn net.Conn)  {
	if tagret.Container != nil {
		if address := getHostAddress(tagret.Container); address != "" {
			address = fmt.Sprintf("%s:%s", address, tagret.Port)
			dp := &tcpproxy.DialProxy{Addr: address}
			dp.HandleConn(conn)
			return;
		}
	}
	conn.Close()
}

func (this *ContainerProxy) start (port string, targetPort string, httpHostPattern string) {
	var p tcpproxy.Proxy

	dContainersTarget := &ContainerByAliasesTarget{containerProxy: this, targetPort: targetPort}
	p.AddHTTPHostMatchRoute(fmt.Sprintf(":%s", port), withHttpHostPattern(httpHostPattern), dContainersTarget)
	for _, selectedTarget := range this.selectedTargets {
		p.AddRoute(fmt.Sprintf(":%s", selectedTarget.Port), selectedTarget)
	}
	fmt.Println(fmt.Sprintf("Start to listen %s port", port))
	log.Fatal(p.Run())
}



func Map(vs []*types.NetworkResource, f func(*types.NetworkResource) string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}


func main() {
	if len(os.Args) < 5 {
		fmt.Println(
		"Usage: docker-container-reverse-proxy [httpHostPattern] [listenPort] [dockerNetworkPattern] [targetContainerPort]")
		fmt.Println(
			"Example: docker-container-reverse-proxy .+\\.my-project.loc 80 my_project_network_[1-9]+ 80")
		return;
	}
	httpHostPattern := os.Args[1]
	port := os.Args[2]
	networkPattern := os.Args[3]
	targetPort := os.Args[4]

	params := os.Args[4:]

	containerProxy := ContainerProxy{
		networkPattern: networkPattern,
		containers: []*types.Container{},
		selectedTargets: []*SelectedContainerTarget{},
	}

	containerProxy.selectedTargets = append(containerProxy.selectedTargets, &SelectedContainerTarget{Name: "ssh", Port: "22"})
	containerProxy.selectedTargets = append(containerProxy.selectedTargets, &SelectedContainerTarget{Name: "postgres", Port: "5432"}) //
	containerProxy.selectedTargets = append(containerProxy.selectedTargets, &SelectedContainerTarget{Name: "mysql", Port: "3306"}) //
	containerProxy.selectedTargets = append(containerProxy.selectedTargets, &SelectedContainerTarget{Name: "mongodb", Port: "27018"})

	containerProxy.createClient()
	containerProxy.loadContainers()
	go func() {
		containerProxy.listen()
	}()

	if contains(params, "--dashboard") {
		go func() {
			dashboard := &Dashboard{}
			dashboard.containerProxy = &containerProxy
			dashboard.start()
		}()
	}
	containerProxy.start(port, targetPort, httpHostPattern)
}

func contains(intSlice []string, searchInt string) bool {
	for _, value := range intSlice {
		if value == searchInt {
			return true
		}
	}
	return false
}