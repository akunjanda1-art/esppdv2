FROM golang:1.22-alpine AS build
WORKDIR /src

COPY shared/go.mod shared/go.sum ./shared/
COPY services/spd-service/go.mod services/spd-service/go.sum ./services/spd-service/

RUN cd services/spd-service && go mod download

COPY shared ./shared
COPY services/spd-service ./services/spd-service

RUN cd services/spd-service && CGO_ENABLED=0 GOOS=linux go build -o /out/spd-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/spd-service /spd-service
EXPOSE 8002
USER nonroot:nonroot
ENTRYPOINT ["/spd-service"]
