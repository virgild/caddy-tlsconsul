FROM caddy:2-builder AS builder
RUN caddy-builder github.com/pteich/caddy-tlsconsul

FROM caddy:2
LABEL maintainer="peter.teich@gmail.com"
LABEL description="Caddy 2 with integrated TLS Consul Storage plugin"
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
