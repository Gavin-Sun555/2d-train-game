APP_NAME := traingame

.PHONY: build run demo test shot clean

## build: 编译可执行文件
build:
	go build -trimpath -o $(APP_NAME) .

## run: 编译并启动
run: build
	./$(APP_NAME)

## demo: 带示例线网启动
demo: build
	./$(APP_NAME) -demo

## test: 运行模拟核心的 headless 测试
test:
	go vet ./...
	go test ./...

## shot: 示例线网快进后截图到 shot.png (WARP 秒)
shot: build
	./$(APP_NAME) -demo -warp $(or $(WARP),120) -shot shot.png -frames 30

clean:
	rm -f $(APP_NAME) shot.png
