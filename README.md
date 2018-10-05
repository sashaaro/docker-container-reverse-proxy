Docker container proxy
===================================

Http proxy for forwarding to local running containers by aliases.
Useful in development environments to reach container services which working on same port.

How use
------
Docker compose example

    version: "3.7"

    services:
      ngserve:
        image: node:8.12-alpine
        entrypoint: /bin/sh -c "([ -d node_modules ] || npm install || (echo sleep 60 && sleep 60)) && ./node_modules/@angular/cli/bin/ng serve --host 0.0.0.0 --port 80 --disable-host-check --proxy-config proxy.conf.json"
        volumes:
          - .:/opt/app
        working_dir: /opt/app
        networks:
          my_project_network:
            aliases:
            - my-project.loc
      nginx:
        image: iginx:latest
        networks:
          my_project_network:
            aliases:
             - api.my-project.loc
    networks:
      my_project_network:
        name: my_project_network

Run

    docker-container-proxy 80 80 my_project_network my-project.loc

Add to /etc/hosts

    my-project.loc      127.0.0.1
    api.my-project.loc  127.0.0.1
   
You can type in browser `my-project.loc`. 