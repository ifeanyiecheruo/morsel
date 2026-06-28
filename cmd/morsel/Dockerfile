FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/morsel-ctrl-plane ./cmd/morsel-ctrl-plane

ARG MORSEL_UID=1000
ARG MORSEL_GID=1000
RUN printf "root:x:0:0::/root:/sbin/nologin\nmorsel:x:${MORSEL_UID}:${MORSEL_GID}::/home/morsel:/sbin/nologin\n" > /out/passwd \
 && printf "root:x:0:\nmorsel:x:${MORSEL_GID}:\n" > /out/group

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/passwd /etc/passwd
COPY --from=builder /out/group /etc/group
COPY --from=builder /out/morsel-ctrl-plane /usr/local/bin/morsel-ctrl-plane
USER morsel
ENTRYPOINT ["morsel-ctrl-plane"]
