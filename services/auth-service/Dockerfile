FROM golang:1.22-alpine AS build
WORKDIR /src

# Workspace files (for local shared module)
COPY go.work ./
COPY shared/go.mod shared/go.sum ./shared/
COPY services/auth-service/go.mod services/auth-service/go.sum ./services/auth-service/

RUN cd services/auth-service && go mod download

COPY shared ./shared
COPY services/auth-service ./services/auth-service

RUN cd services/auth-service && CGO_ENABLED=0 GOOS=linux go build -o /out/auth-service ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /
COPY --from=build /out/auth-service /auth-service
EXPOSE 8001
USER nonroot:nonroot
ENTRYPOINT ["/auth-service"]

