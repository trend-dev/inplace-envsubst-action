FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build

WORKDIR /src/
COPY go.mod /src/
COPY *.go /src/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/envsubst .

FROM scratch
COPY --from=build /bin/envsubst /bin/envsubst
ENTRYPOINT ["/bin/envsubst"]
