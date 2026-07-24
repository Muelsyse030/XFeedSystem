APP_NAME := xfeed-api
BUILD_DIR := build
MAIN_PATH := ./cmd/api
PKG_NAME := $(APP_NAME)-$(shell date +%Y%m%d-%H%M%S).tar.gz

.PHONY: help run build clean test lint migrate seed docker-up docker-down docker-migrate docker-build docker-up-prod package deploy

help: ## 显示帮助信息
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

run: ## 直接运行服务
	go run $(MAIN_PATH)/main.go

build: ## 编译二进制文件
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)/main.go

clean: ## 清理编译产物
	rm -rf $(BUILD_DIR)

package: build ## 编译并打包为部署压缩包
	@mkdir -p $(BUILD_DIR)/pkg/$(APP_NAME)/configs
	@mkdir -p $(BUILD_DIR)/pkg/$(APP_NAME)/migrations
	@mkdir -p $(BUILD_DIR)/pkg/$(APP_NAME)/scripts
	@mkdir -p $(BUILD_DIR)/pkg/$(APP_NAME)/deploy
	cp $(BUILD_DIR)/$(APP_NAME) $(BUILD_DIR)/pkg/$(APP_NAME)/
	cp configs/config.yaml $(BUILD_DIR)/pkg/$(APP_NAME)/configs/
	cp migrations/*.sql $(BUILD_DIR)/pkg/$(APP_NAME)/migrations/
	cp scripts/docker-compose.yaml $(BUILD_DIR)/pkg/$(APP_NAME)/scripts/
	cp deploy/xfeed-api.service $(BUILD_DIR)/pkg/$(APP_NAME)/deploy/
	cp deploy/install.sh $(BUILD_DIR)/pkg/$(APP_NAME)/deploy/
	cp .env.example $(BUILD_DIR)/pkg/$(APP_NAME)/
	chmod +x $(BUILD_DIR)/pkg/$(APP_NAME)/$(APP_NAME)
	chmod +x $(BUILD_DIR)/pkg/$(APP_NAME)/deploy/install.sh
	cd $(BUILD_DIR)/pkg && tar czf ../$(PKG_NAME) $(APP_NAME)
	@echo ""
	@echo "✓ 打包完成: $(BUILD_DIR)/$(PKG_NAME)"
	@ls -lh $(BUILD_DIR)/$(PKG_NAME)

deploy: package ## 打包并上传到服务器（需设置 SERVER 变量，如 make deploy SERVER=root@1.2.3.4）
	@if [ -z "$(SERVER)" ]; then echo "错误: 请设置 SERVER 变量，如 make deploy SERVER=root@1.2.3.4"; exit 1; fi
	scp $(BUILD_DIR)/$(PKG_NAME) $(SERVER):/tmp/
	ssh $(SERVER) "\
		cd /tmp && tar xzf $(PKG_NAME) && \
		cd $(APP_NAME) && \
		bash deploy/install.sh update && \
		cd /tmp && rm -rf $(APP_NAME) $(PKG_NAME)"
	@echo "✓ 已部署到 $(SERVER)，服务已自动重启"

test: ## 运行测试
	go test -v -race -count=1 ./...

test-cover: ## 运行测试并输出覆盖率
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint: ## 代码检查 (需要 golangci-lint)
	golangci-lint run ./...

fmt: ## 格式化代码
	go fmt ./...

vet: ## 静态分析
	go vet ./...

tidy: ## 整理依赖
	go mod tidy

download: ## 下载依赖
	go mod download

migrate: ## 运行数据库迁移 (需提供 DB_HOST 等环境变量)
	@for f in migrations/*.sql; do \
		echo "Running $$f..."; \
		mysql -h$${DB_HOST:-localhost} -P$${DB_PORT:-3306} -u$${DB_USER:-root} -p$${DB_PASS} $${DB_NAME:-feed_system} < $$f; \
	done

docker-migrate: ## 通过 Docker 容器运行数据库迁移
	@for f in migrations/*.sql; do \
		echo "Running $$f..."; \
		docker exec -i feed_mysql mysql -uroot -p$${MYSQL_ROOT_PASSWORD:-123456} feed_system < $$f; \
	done

docker-build: ## 构建 API Docker 镜像
	docker build -t xfeed-api:latest .

docker-up-prod: ## 启动生产环境 (需要 .env)
	docker-compose -f deploy/docker-compose.prod.yaml --env-file .env up -d

seed: ## 填充测试数据
	go run scripts/seed_notes.go

docker-up: ## 启动 Docker 服务 (MySQL + Redis)
	docker-compose -f scripts/docker-compose.yaml up -d

docker-down: ## 停止 Docker 服务
	docker-compose -f scripts/docker-compose.yaml down

dev: docker-up run ## 启动开发环境 (Docker + 服务)
