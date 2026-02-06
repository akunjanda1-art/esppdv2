FROM golang:1.22-alpine AS build
WORKDIR /src

COPY shared/go.mod shared/go.sum ./shared/
COPY services/notification-service/go.mod services/notification-service/go.sum ./services/notification-service/

RUN cd services/notification-service && go mod download

COPY shared ./shared
COPY services/notification-service ./services/notification-service

RUN cd services/notification-service && CGO_ENABLED=0 GOOS=linux go build -o /out/notification-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/notification-service /notification-service
EXPOSE 8005
USER nonroot:nonroot
ENTRYPOINT ["/notification-service"]
