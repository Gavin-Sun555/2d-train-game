APP_NAME := traingame

.PHONY: build run test shot clean

## build: 编译可执行文件
build:
	go build -trimpath -o $(APP_NAME) .

## run: 编译并启动 (SCENARIO=1|2)
run: build
	./$(APP_NAME) -scenario $(or $(SCENARIO),1)

## test: 运行模拟核心的 headless 测试
test:
	go vet ./...
	go test ./...

## shot: 快进后截图到 shot.png (SCENARIO, WARP 秒)
shot: build
	./$(APP_NAME) -scenario $(or $(SCENARIO),2) -warp $(or $(WARP),120) -shot shot.png -frames 30

clean:
	rm -f $(APP_NAME) shot.png
