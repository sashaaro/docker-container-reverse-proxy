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

	networkNames := []string{};
	for _, network := range networks {
		matched, err := regexp.MatchString(this.networkPattern, network.Name)
		if err != nil {
			fmt.Printf(err.Error());
			return;
		}

		if (matched) {
			networkNames = append(networkNames, network.Name);
		}
	}

	// fmt.Println("aliases %s", len(containers[0].NetworkSettings.Networks["my_network"].Aliases))
	// get aliases and fill
	for _, container := range containers {
		for _, networkName := range networkNames {
			if network, ok := container.NetworkSettings.Networks[networkName]; ok {
				con, _ := this.cli.ContainerInspect(context.Background(), container.ID)
				network.Aliases = con.NetworkSettings.Networks[networkName].Aliases

				this.containers = append(this.containers, &container);
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
		return
	}
	if wrap.HostName == "" {
		return;
	}

	for _, container := range ch.containerProxy.containers {
		for _, network := range container.NetworkSettings.Networks {
			for _, alias := range network.Aliases {
				if strings.Index(wrap.HostName, alias) == 0 { //with any port
					hNetwork, ok := container.NetworkSettings.Networks[container.HostConfig.NetworkMode]
					if ok { // sometime while "network:disconnect" event fire
						if hNetwork.IPAddress != "" {
							addr := fmt.Sprintf("%s:%s", hNetwork.IPAddress, ch.targetPort)
							fmt.Printf("Forwarding %s to %s\n", wrap.HostName, addr)
							dp := &tcpproxy.DialProxy{Addr: addr}
							dp.HandleConn(conn)
							return
						} else {
							// log
						}
					} else {
						// return nil, fmt.Errorf("unable to find network settings for the network %s", networkMode)
					}
				}
			}
		}
	}
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

func (this *ContainerProxy) start (port string, targetPort string, httpHostPattern string) {
	var p tcpproxy.Proxy

	dContainersTarget := &ContainerByAliasesTarget{containerProxy: this, targetPort: targetPort}
	p.AddHTTPHostMatchRoute(fmt.Sprintf(":%s", port), withHttpHostPattern(httpHostPattern), dContainersTarget)
	fmt.Println(fmt.Sprintf("Start to listen %s port", port))
	log.Fatal(p.Run())
}

func main() {
	if len(os.Args) < 5 {
		fmt.Println(
		"Usage: docker-container-proxy [httpHostPattern] [listenPort] [dockerNetworkPattern] [targetContainerPort]")
		fmt.Println(
			"Example: docker-container-proxy .+\\.my-project.loc 80 my_project_network_[1-9]+ 80")
		return;
	}
	httpHostPattern := os.Args[1]
	port := os.Args[2]
	networkPattern := os.Args[3]
	targetPort := os.Args[4]

	containerProxy := ContainerProxy{
		networkPattern: networkPattern,
		containers: []*types.Container{},
	}

	containerProxy.createClient()
	containerProxy.loadContainers()
	go func() {
		containerProxy.listen()
	}()

	containerProxy.start(port, targetPort, httpHostPattern)
}