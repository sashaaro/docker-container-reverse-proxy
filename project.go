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
	"strings"
	"time"
)

type ContainerProxy struct {
	containers []types.Container
	cli *client.Client
}



const networkName = "skyeng"//"my_network"
const postfixDomain = ".skyeng.loc"

func (this *ContainerProxy) createClient () {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))

	if err != nil {
		panic(err)
		return;
	}
	this.cli = cli

}

func (this *ContainerProxy) loadContainers () {
	fmt.Println("loadContainers")
	containers, err := this.cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		fmt.Printf(err.Error());
	} else {
		// fmt.Println("aliases %s", len(containers[0].NetworkSettings.Networks["my_network"].Aliases))

		this.containers = containers

		// get aliases
		for _, container := range this.containers {
			con, _ := this.cli.ContainerInspect(context.Background(), container.ID)
			container.NetworkSettings.Networks[networkName].Aliases =
			con.NetworkSettings.Networks[networkName].Aliases
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
			/*fmt.Printf("Network: %v", n)
			fmt.Printf("IPAddress: %v", network.IPAddress)
			fmt.Printf("Aliases: %v", network.Aliases)*/
			for _, alias := range network.Aliases {
				//fmt.Printf("Alias: %s \n", alias)
				//fmt.Printf("wrap.HostName: %s \n", wrap.HostName)

				if strings.Index(wrap.HostName, alias) == 0 { //with any port
					hNetwork, ok := container.NetworkSettings.Networks[container.HostConfig.NetworkMode]
					if ok { // sometime while "network:disconnect" event fire
						if hNetwork.IPAddress != "" {
							addr := fmt.Sprintf("%s:4210", hNetwork.IPAddress)
							fmt.Printf("Addr: %s \n", addr)
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

func withPostfixDomain(domain string) tcpproxy.Matcher {
	return func(_ context.Context, got string) bool {
		fmt.Printf("Get: %s \n", got)
		return strings.Index(got, domain) != -1
	}
}

func (this *ContainerProxy) start () {
	var p tcpproxy.Proxy

	target := &ContainerByAliasesTarget{containerProxy: this}
	p.AddHTTPHostMatchRoute(":4299", withPostfixDomain(postfixDomain), target)
	log.Fatal(p.Run())
}

func main() {
	containerProxy := ContainerProxy{}

	containerProxy.createClient()
	containerProxy.loadContainers()
	go func() {
		containerProxy.listen()
	}()
	containerProxy.start()
}