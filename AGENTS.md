# Go 项目模板

基于内置 aikit 工具包（`pkg/aikit/`）的 Go 后端服务项目模板，快速搭建生产级 Go HTTP 服务。

## 核心功能

- **功能 1** - 功能描述
- **功能 2** - 功能描述

## 技术栈

- **前端**: React 19, React Router v7, SWR, Zustand, Zod, Tailwind CSS v4, shadcn/ui, axios
- **后端**: Go 1.25+, Gin, GORM (MySQL), go-redis/v9, aikit (`pkg/aikit/`)
- **测试工具**: testify, Vitest, agent-browser (基于 playwright)
- **代码检查**: Biome, golangci-lint

## 命令

```bash
./run.sh install            # 安装后端 + 前端依赖
./run.sh start              # 启动后端 + 前端
./run.sh stop               # 停止所有服务
./run.sh restart            # 重启所有服务
./run.sh status             # 查看状态
./run.sh backend start|stop # 仅操作后端
./run.sh frontend start|stop# 仅操作前端
./run.sh build              # 编译后端二进制
./run.sh migrate            # 执行数据库迁移
./run.sh test               # 运行后端测试
./run.sh lint               # 代码检查（golangci-lint）
./run.sh swagger            # 生成 Swagger 文档
```

## 相关文档

在处理特定领域的工作时，请阅读以下文档或者使用以下技能：

| Document or Skill | When to Read or Use |
|-------------------|---------------------|
| http://127.0.0.1:{port}/swagger/index.html | Swagger API 文档（{port} 替换为实际端口） |
| http://127.0.0.1:{port}/healthz | 健康检查接口 |
| http://127.0.0.1:5173 | 前端开发服务器 |
| backend/AGENTS.md | 后端开发规范（分层架构、代码规范、测试规范等） |
| frontend/AGENTS.md | 前端开发规范（React Router v7、SWR、Zustand 等） |
| backend/pkg/aikit/*/README.md | 查阅内置 aikit 子模块用法 |
| /plugin:context7:context7 | 获取开源库最新文档 |
| /git-commit | 提交代码 |

## 注意事项

- `backend/internal/api/article.go` 是示例实现，参考该示例开发其他功能
- 开发步骤：**小功能完整闭环迭代**，测试驱动开发 → 完整功能实现 → review → 提交 → 重启服务 → 人工 check → 再做下一个功能
- 新增表结构变更时，在 `backend/cmd/migrate/migrations/` 下创建迁移文件，然后执行 `./run.sh migrate`
- 每一次我的指令你觉得模棱两可，向我提问，明确需求再行动

---

## 编程行为准则

Behavioral guidelines to reduce common LLM coding mistakes.

**Tradeoff:** 这些准则偏向谨慎而非速度。对于简单任务，请自行判断。

### 1. Think Before Coding

**不要假设。不要隐藏困惑。展示权衡。**

在实现之前：
- 明确陈述你的假设。如果不确定，先问。
- 如果存在多种解释，都展示出来——不要默默选择一个。
- 如果存在更简单的方案，指出它。在合理时提出反对意见。
- 如果有不清楚的地方，停下来。说出什么让你困惑。先问清楚。

### 2. Simplicity First

**最小化代码解决问题。不做 speculation 的工作。**

- 不添加需求之外的功能。
- 不为单次使用的代码创建抽象。
- 不添加未被请求的"灵活性"或"可配置性"。
- 不处理不可能发生的场景的错误。
- 如果你写了200行而可以写成50行，重写。

问自己："一个资深工程师会说这太复杂了吗？"如果是，简化。

### 3. Surgical Changes

**只触碰必须改的地方。只清理自己的烂摊子。**

编辑现有代码时：
- 不要"改进"相邻的代码、注释或格式。
- 不要重构没坏的东西。
- 匹配现有风格，即使你会有不同做法。
- 如果注意到无关的死代码，指出它——不要删除它。

当你的改动造成孤立代码时：
- 移除因你的改动而变得未使用的 imports/变量/函数。
- 不要删除先前存在的死代码，除非被要求。

检验标准：每一行改动都应该直接源于用户的要求。

### 4. Goal-Driven Execution

**定义成功标准。循环验证直到完成。**

将任务转化为可验证的目标：
- "添加验证" → "为无效输入写测试，然后让它们通过"
- "修复 bug" → "写一个复现 bug 的测试，然后让它通过"
- "重构 X" → "确保测试在重构前后都通过"

对于多步骤任务，陈述一个简要计划：
```
1. [步骤] → 验证: [检查点]
2. [步骤] → 验证: [检查点]
3. [步骤] → 验证: [检查点]
```

强的成功标准让你能独立循环。弱的标准（"让它能工作"）需要不断确认。

---

**这些准则生效的表现：** diff 中不必要的改动减少、因过度复杂导致的返工减少、澄清性问题出现在错误之前而非之后。
