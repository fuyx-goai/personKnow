# 项目部署笔记

# 服务器初始化检查清单

新拿到一台 Linux 服务器，先做四件事：
1. 更新系统软件源并升级基础包；
2. 创建普通用户并禁用 root 直接 SSH 登录；
3. 配置 SSH 密钥登录，关闭密码登录更安全；
4. 开启防火墙，只放行需要的端口（如 22、80、443）。

# Docker 部署应用的步骤

用 Docker 部署 Go 应用推荐多阶段构建：
第一阶段用 golang 镜像编译出二进制文件，
第二阶段用极小的 alpine 或 scratch 镜像只携带二进制文件运行，
最终镜像体积可以控制在 20MB 以内。
运行时注意容器内的时区要设为 Asia/Shanghai，否则日志时间会差 8 小时。

# Nginx 反向代理配置要点

Nginx 做反向代理时，`proxy_pass` 指向后端真实服务地址，
记得用 `proxy_set_header Host $host` 把原始域名传给后端，
用 `proxy_set_header X-Real-IP $remote_addr` 传递真实客户端 IP，
否则后端日志里记录的全是 Nginx 的内网地址。
