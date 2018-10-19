Docker container reverse proxy
===================================

Reverse http proxy for forwarding to local running containers by [network aliases](https://docs.docker.com/v17.09/engine/userguide/networking/configure-dns).
Useful in development environments to reach container services which working on same port.

Compile `go build .`

How use
------

    docker-container-reverse-proxy [httpHostPattern] [listenPort] [dockerNetworkPattern] [targetContainerPort] --dashboard

```docker-compose.yml``` example with services are working on same 80 port.

    version: "3.7"

    services:
      frontend:
        image: node:8.12-alpine
        entrypoint: /bin/sh -c "npm install && ./node_modules/@angular/cli/bin/ng serve --port 80 --host my-project.loc"
        volumes:
          - .:/opt/app
        working_dir: /opt/app
        networks:
          my_project_network:
            aliases:
            - my-project.loc
      api:
        image: nginx:latest
        networks:
          my_project_network:
            aliases:
             - api.my-project.loc
      payment-api:
        image: nginx:latest
        networks:
          my_project_network:
            aliases:
              - payment.my-project.loc
    networks:
      my_project_network: ~

Probably in real project some services will located in separate docker-compose.yml file, but
there is not problem if they have same network or [dockerNetworkPattern] argument covers necessary networks

Example

    docker-container-proxy .+\.my-project\.loc 80 my_project_network 80

Don't forgot edit your ```/etc/hosts``` accordingly

    my-project.loc          127.0.0.1
    api.my-project.loc      127.0.0.1
    payment.my-project.loc  127.0.0.1
   
You can type in browser `my-project.loc` and `api.my-project.loc`.

```---dashboard``` flag runs dashboard on 8080 for setup tcp proxy to another ports.(in progress)

Similar projects:
[jwilder/nginx-proxy](https://github.com/jwilder/nginx-proxy)
[traefik](https://traefik.io)

More advanced way to reach to container without expose ports is locally setup dns server
[coredns-dockerdiscovery](https://github.com/kevinjqiu/coredns-dockerdiscovery)