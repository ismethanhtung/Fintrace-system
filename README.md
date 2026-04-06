# Fintrace-system

docker logs -f fintrace-worker

docker exec -it fintrace-redis redis-cli

DOCKER_BUILDKIT=0 COMPOSE_DOCKER_CLI_BUILD=0 docker-compose up -d --build

docker run --rm -v "$PWD/worker":/app -w /app golang:1.22-alpine go mod tidy
-> cái này chạy ở thư mục gốc
