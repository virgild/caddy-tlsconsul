FROM caddy:2.0.0-builder AS builder
RUN caddy-builder github.com/rgdev/caddy-tlsconsul

FROM caddy:2.0.0
LABEL maintainer="peter.teich@gmail.com"
LABEL description="Caddy 2 with integrated TLS Consul Storage plugin"
COPY --from=builder /usr/bin/caddy /usr/bin/caddy