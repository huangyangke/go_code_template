# 前端项目规范

## 渲染模式
采用 CSR（Client-Side Rendering）+ SPA 模式，构建纯前端 SPA。

## 技术栈
- **框架**: React 19 + React Router v7
- **样式**: Tailwind CSS v4 + shadcn/ui
- **数据获取**: SWR（GET 请求）+ axios（mutations / 非缓存请求）
- **状态管理**: Zustand（全局状态）、React state（局部状态）
- **校验**: Zod
- **构建**: Vite 8
- **代码检查**: Biome
- **测试**: Vitest + Testing Library

## 目录结构

```
app/
├── routes/              # React Router v7 文件路由（框架约定）
├── components/          # 全局通用组件
│   └── ui/              # shadcn/ui 组件（npx shadcn add 自动生成）
├── hooks/               # 全局自定义 hooks
├── stores/              # Zustand stores
├── services/            # API 层：axios 实例 + SWR fetcher
├── lib/                 # 工具函数、常量、Zod schemas
├── types/               # 全局 TypeScript 类型
├── test/                # 测试配置（setup.ts）
├── root.tsx             # 根布局 + SWRConfig provider
├── routes.ts            # 路由注册
└── app.css              # 全局样式 + Tailwind 入口
```

### 页面级 Co-location

当页面逻辑较复杂时，在 `routes/` 下建页面子目录：

```
routes/<page>/
├── components/          # 页面专属组件
├── hooks/               # 页面专属 hooks
├── api.ts               # 页面专属 API 调用
├── types.ts             # 页面专属类型
├── constants.ts         # 页面专属常量
└── index.tsx            # 页面入口（路由组件）
```

简单页面直接用单文件 `routes/<page>.tsx`。

## 命名约定

| 类型 | 命名 | 示例 |
|------|------|------|
| 组件 | PascalCase | `UserCard.tsx` |
| hooks | `use` 前缀 | `useAuth.ts` |
| stores | `use...Store` | `useUserStore.ts` |
| 工具函数 | camelCase | `formatDate.ts` |
| 类型文件 | camelCase | `api.ts` |

## API 调用模式

### GET 请求 — 用 SWR

```tsx
import useSWR from "swr";

const { data, error, isLoading } = useSWR<ArticleList>("/v1/article/list?page=1");
```

SWR 全局 fetcher 已在 `root.tsx` 通过 `SWRConfig` 配置，自动使用 `services/fetcher.ts` 中的 `swrFetcher`。

### 写操作 / 非缓存请求 — 用 axios

```tsx
import apiClient from "~/services/api";

const res = await apiClient.post("/v1/article/add", { title: "..." });
```

### 后端响应格式

```ts
{ code: 200, data: T, msg: "" }  // code === 200 表示成功
```

## 组件规范

- 使用 shadcn/ui 作为基础组件库，通过 `npx shadcn add <component>` 添加
- 样式合并使用 `cn()` 函数（`~/lib/utils.ts`）
- 优先使用 Tailwind CSS class，避免内联 style

## 状态管理

- **局部状态**: `useState` / `useReducer`
- **全局状态**: Zustand store（放在 `stores/` 目录）
- **服务端数据**: SWR 管理（不放 store）

## 相关文档

| Document or Skill | When to Read or Use |
|-------------------|---------------------|
| http://127.0.0.1:{port}/swagger/index.html | Swagger API 文档（{port} 替换为实际运行端口） |
| node_modules/react-router/dist/ | React Router v7 官方类型与实现 |
| https://reactrouter.com/7.14.2/home | React Router v7 官方文档 |
| ui-ux-pro-max-skill | 设计 UI、UX 时使用此技能 |

## 注意事项

- 所有 React Router v7 相关实现，以 `node_modules/react-router/` 源码和官方文档为最高优先级，不沿用旧版（v5/v6）经验直接写代码
- 遇到 API 变更、弃用提示或行为差异时，按当前版本文档修正
- 涉及后端联调时，以 OpenAPI 文档为准，不手写猜测请求参数、响应结构或字段命名
- type-only imports 必须使用 `import type`（tsconfig 开启了 `verbatimModuleSyntax`）
