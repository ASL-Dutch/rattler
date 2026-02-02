# Rattler Makefile - 交叉编译 Linux/Windows 64 位二进制与部署打包

BINARY_NAME := rattler
BUILD_DIR := build
DEPLOY_DIR := $(BUILD_DIR)/rattler-deploy-windows-amd64
DEPLOY_CADDY_DIR := $(DEPLOY_DIR)/caddy


# Go 编译参数 (-s -w 减小二进制体积)
LDFLAGS := -ldflags "-s -w"

.PHONY: all build build-linux build-windows build-all clean deploy-pack deploy-pack-caddy deploy-pack-nssm deploy-clean

# 默认目标：编译当前平台
all: build

# 编译当前平台
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# 编译 Linux 64 位 (amd64)
build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

# 编译 Windows 64 位 (amd64)
build-windows:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# 同时编译 Linux 和 Windows 64 位
build-all: build-linux build-windows
	@echo "Done. Binaries in $(BUILD_DIR)/:"
	@ls -la $(BUILD_DIR)/

# 清理编译产物
clean:
	rm -rf $(BUILD_DIR)

# ========== 部署打包 ==========
# 将 Caddy、NSSM、项目二进制、config.yaml 模版 打包到 dist/rattler-deploy-windows-amd64/
# 依赖: curl, unzip (macOS 自带)

deploy-pack: deploy-clean build-windows deploy-pack-files
	@echo ""
	@echo "=== 部署包已生成 ==="
	@echo "目录: $(DEPLOY_DIR)"
	@echo "内容: rattler.exe, config.yaml, Caddyfile, caddy.exe, nssm.exe, DEPLOY.md, CADDY_README.md"
	@ls -la $(DEPLOY_DIR)/

# 打包项目二进制与配置文件模版
deploy-pack-files:
	@mkdir -p $(DEPLOY_DIR)
	@mkdir -p $(DEPLOY_CADDY_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(DEPLOY_DIR)/rattler.exe
	@cp config.yaml $(DEPLOY_DIR)/config.yaml
	@cp Caddyfile $(DEPLOY_CADDY_DIR)/Caddyfile
	@cp DEPLOY.md $(DEPLOY_DIR)/DEPLOY.md
	@cp CADDY_README.md $(DEPLOY_DIR)/CADDY_README.md
	@echo "[deploy] rattler.exe, config.yaml, Caddyfile, 文档已复制"

# 清理部署包
deploy-clean:
	@echo "[deploy] 清理部署包"
	rm -rf $(DEPLOY_DIR)
