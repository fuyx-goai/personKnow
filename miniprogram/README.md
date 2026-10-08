# 个人知识库微信小程序

这是当前 Go 知识库服务的微信原生小程序客户端，页面结构与视觉来自 Figma「个人知识库小程序」原型。

## 已接入能力

- 微信 `wx.login` 登录、访问令牌自动刷新与安全退出；
- 私有/公开知识库、分类搜索、创建、删除和公开只读访问；
- 七类文件上传、搜索、在线编辑、重命名、重新索引和异步删除；
- 多知识库问答、会话历史、流式回答、引用溯源和 Token 信息；
- 存储/Token 用量、RAG 检索参数、设备会话、审计日志与 JSON 备份。

## 微信开发者工具

1. 打开微信开发者工具，选择“导入项目”；
2. 项目目录选择本 `miniprogram` 文件夹；
3. 本地预览可使用 `touristappid`，真机调试或发布前替换 `project.config.json` 中的 `appid`；
4. 开发阶段可在详情设置中关闭合法域名校验；
5. 进入“我的空间”，将服务地址改为实际 Go 网关地址。

默认接口地址为 `http://127.0.0.1:8080`。开发者工具访问的是本机，真机中的 `127.0.0.1` 指向手机自身，必须改为手机可访问的局域网地址或 HTTPS 公网域名。

## 发布前配置

- 在微信公众平台分别配置 `request`、`uploadFile`、`downloadFile` HTTPS 合法域名；
- 生产环境必须使用 HTTPS；
- 确认网关可访问 `/api/health`、`/api/config` 与 `/api/v1/*`；
- 配置微信登录所需的 AppID、AppSecret 与网关合法域名；
- 反向代理需关闭 SSE 缓冲并允许长连接，否则流式问答会退化为一次性返回；
- 将 `project.config.json` 中的 `urlCheck` 恢复为 `true`；
- 使用真实小程序 AppID 重新编译，并真机验证登录、七类文件上传、问答引用、Web 扫码确认和退出登录。

微信私有项目配置必须放在 `project.private.config.json`，该文件已被 Git 忽略，禁止提交 AppSecret 或其他凭证。

## 验证命令

```bash
cd miniprogram
node --test tests/*.test.js
find . -name '*.js' -not -path './tests/*' -print0 | xargs -0 -n1 node --check
```
