# Rattler Makefile — 编译与部署打包

BINARY   := rattler
BUILD_DIR := build
DIST_DIR  := dist
DEPLOY_PKG := rattler-deploy-windows-amd64
DEPLOY_PATH := $(DIST_DIR)/$(DEPLOY_PKG)
CADDY_PATH := $(DEPLOY_PATH)/caddy

LDFLAGS  := -ldflags "-s -w"

.PHONY: all build linux win cross clean pack clean-pack copy-deploy zip-deploy help

# 默认：当前平台
all: build

help:
	@echo "Rattler Makefile"
	@echo "  make          / make build   — 编译当前平台 → $(BUILD_DIR)/"
	@echo "  make linux    — Linux amd64 → $(BUILD_DIR)/$(BINARY)-linux-amd64"
	@echo "  make win      — Windows amd64 → $(BUILD_DIR)/$(BINARY)-windows-amd64.exe"
	@echo "  make cross    — 同时编译 linux + win"
	@echo "  make pack     — 打 Windows 部署包 → $(DIST_DIR)/$(DEPLOY_PKG).zip"
	@echo "  make clean    — 删除 $(BUILD_DIR)/ 与 $(DIST_DIR)/"

# ——— 编译 ———
build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) .

linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 .

win:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe .

cross: linux win
	@echo "→ $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/

# ——— 清理 ———
clean:
	rm -rf $(BUILD_DIR) $(DIST_DIR)

# ——— 部署打包（Windows：二进制 + 配置 + Caddy + 文档） ———
pack: clean-pack win copy-deploy zip-deploy
	@echo ""
	@echo "→ 部署包: $(DEPLOY_PATH).zip"
	@ls -la $(DIST_DIR)/

clean-pack:
	rm -rf $(DEPLOY_PATH) $(DEPLOY_PATH).zip

copy-deploy:
	@mkdir -p $(DEPLOY_PATH) $(CADDY_PATH)
	@cp $(BUILD_DIR)/$(BINARY)-windows-amd64.exe $(DEPLOY_PATH)/rattler.exe
	@cp config.yaml $(DEPLOY_PATH)/config.yaml
	@cp Caddyfile $(CADDY_PATH)/Caddyfile
	@cp DEPLOY.md $(DEPLOY_PATH)/DEPLOY.md
	@cp CADDY_README.md $(DEPLOY_PATH)/CADDY_README.md

zip-deploy:
	@cd $(DIST_DIR) && zip -r $(DEPLOY_PKG).zip $(DEPLOY_PKG)
