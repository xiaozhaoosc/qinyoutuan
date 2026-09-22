# 亲友团 · 自动化测试结果

- 执行时间：20260922-160729
- 主机：instance-20260921-0904
- Go：go version go1.25.0 linux/arm64
- GOPROXY：https://goproxy.cn,direct
- 参数：--no-frontend --no-smoke

## 步骤结果

- [x] go build ./...
  - 耗时：0m05s
- [x] go vet ./...
  - 耗时：0m03s
- [x] go test -race -count=1 ./...
  - ok 包 18 个；无测试文件包 7 个；总耗时 9m05s
- [x] 测试用例清单已生成 → docs/test-report/test-cases.md
  - 耗时：0m13s
- [x] bash -n 三脚本语法
  - 耗时：0m00s
- [x] test-argo.sh（Argo 专项，跳过其自带冒烟，统一走本脚本冒烟）
  - 耗时：0m03s
- (跳过) 前端三步（--no-frontend 跳过）
- (跳过) 启动冒烟（--no-smoke 跳过）

## 汇总

- 通过：6 项；失败：0 项；跳过：2 项
- 总耗时：9m29s
- 测试用例清单：docs/test-report/test-cases.md
- 结论：✅ 全部通过
