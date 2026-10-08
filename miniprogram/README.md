# 个人知识库微信小程序

这是当前 Go 知识库服务的微信原生小程序客户端，页面结构与视觉来自 Figma「个人知识库小程序」原型。

## 已接入能力

- 流式知识问答、停止生成和引用溯源；
- 知识片段统计与来源聚合；
- 原资料搜索、服务端路径摄入和按来源删除；
- 服务状态、模型配置与 API 地址管理。

原型中的多知识库、微信登录、Token 用量、客户端文件上传、在线编辑和重新索引缺少后端接口。相关入口会明确提示暂未开放，不会返回模拟成功结果。

## 微信开发者工具

1. 打开微信开发者工具，选择“导入项目”；
2. 项目目录选择本 `miniprogram` 文件夹；
3. 本地预览可使用 `touristappid`，真机调试或发布前替换 `project.config.json` 中的 `appid`；
4. 开发阶段可在详情设置中关闭合法域名校验；
5. 进入“我的空间”，将服务地址改为实际 Go 网关地址。

默认接口地址为 `http://127.0.0.1:8080`。开发者工具访问的是本机，真机中的 `127.0.0.1` 指向手机自身，必须改为手机可访问的局域网地址或 HTTPS 公网域名。

## 发布前配置

- 在微信公众平台配置 `request` 合法域名；
- 生产环境必须使用 HTTPS；
- 确认网关可访问 `/api/health`、`/api/config`、`/api/stats`、`/api/chunks`、`/api/ingest`、`/api/chat/stream`；
- 将 `project.config.json` 中的 `urlCheck` 恢复为 `true`；
- 使用真实小程序 AppID 重新编译和真机验证。

## 验证命令

```bash
cd miniprogram
node --test tests/*.test.js
find . -name '*.js' -not -path './tests/*' -print0 | xargs -0 -n1 node --check
```
