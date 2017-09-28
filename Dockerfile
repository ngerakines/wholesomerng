FROM alpine:latest

MAINTAINER Nick Gerakines <nick@gerakines.net>

WORKDIR "/opt"

ADD .docker_build/wholesomerng /opt/bin/wholesomerng
ADD ./data.txt /opt/data.txt

CMD ["/opt/bin/wholesomerng", "-address=$PORT", "-source=/opt/data.txt"]
