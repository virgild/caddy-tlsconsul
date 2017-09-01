FROM golang:1.8

ENV CADDY_VERSION 0.10.7

RUN go get -d github.com/mholt/caddy && go get -d github.com/pteich/caddy-tlsconsul \
    && cd /go/src/github.com/mholt/caddy &&  git checkout tags/v${CADDY_VERSION}

RUN sed -e "s#// This is where other plugins get plugged in (imported)#_ \"github.com/pteich/caddy-tlsconsul\"#" -i /go/src/github.com/mholt/caddy/caddy/caddymain/run.go

WORKDIR /go/src/github.com/mholt/caddy/caddy
RUN CGO_ENABLED=0 GOOS=linux bash build.bash

FROM alpine:latest
LABEL maintainer="peter.teich@gmail.com"
LABEL description="Caddy with integrated TLS Consul Storage plugin"

ENV DUMBINIT_VERSION 1.2.0
ENV CADDYPATH /.caddy

RUN set -x \
    && apk update && apk add --no-cache \
        openssl \
        dpkg \
        ca-certificates \
    && update-ca-certificates \
    && cd /tmp \
    && dpkgArch="$(dpkg --print-architecture | awk -F- '{ print $NF }')" \
    && wget -O /usr/local/bin/dumb-init "https://github.com/Yelp/dumb-init/releases/download/v${DUMBINIT_VERSION}/dumb-init_${DUMBINIT_VERSION}_${dpkgArch}" \
    && chmod +x /usr/local/bin/dumb-init \
    && rm -rf /tmp/*

COPY --from=0 /go/src/github.com/mholt/caddy/caddy/caddy /bin/caddy
RUN chmod +x /bin/caddy

ENTRYPOINT ["/usr/local/bin/dumb-init","/bin/caddy"]

EXPOSE 80 443 2015
WORKDIR /var/www/html
CMD [""]
