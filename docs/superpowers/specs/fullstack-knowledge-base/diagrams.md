# 个人知识库全栈版本 · 图表文档

## 1. 系统思维导图（功能全貌）

图1：个人知识库功能全貌思维导图。四个原型页面由账号、权限、索引、配额和日志能力共同支撑。

```mermaid
mindmap
  root((个人知识库))
    问答对答
      指定知识库
      全域检索
      流式回答
      引用溯源
    知识库体系
      多知识库
      公开与私有
      分类搜索
      统计概览
    原资料操作
      七类文件上传
      解析文本编辑
      重新索引
      彻底删除
    我的空间
      微信登录
      Web扫码登录
      容量与Token
      RAG检索微调
      设备会话安全
      账号退出
    系统能力
      PostgreSQL任务
      Mem与Milvus
      审计与运行日志
      旧数据迁移
```

## 2. 核心业务流程图

图2：用户从登录到可溯源问答的主流程。上传和索引解耦，索引失败不影响旧版本问答。

```mermaid
flowchart TD
    A([开始]) --> B[打开小程序或Web]
    B --> C{是否已登录}
    C -- 否 --> D[微信登录或扫码确认]
    D --> E{登录成功}
    E -- 否 --> F[展示错误并允许重试] --> D
    E -- 是 --> G[进入原型四页导航]
    C -- 是 --> G
    G --> H{选择操作}
    H -- 管理知识库 --> I[创建或选择知识库]
    H -- 管理资料 --> J[上传或编辑资料]
    J --> K{权限与配额通过}
    K -- 否 --> L[展示权限或配额错误] --> G
    K -- 是 --> M[创建异步索引任务]
    M --> N{索引成功}
    N -- 否 --> O[保留旧版本并允许重试] --> G
    N -- 是 --> P[切换有效向量版本]
    H -- 发起问答 --> Q[选择单库或全域模式]
    I --> Q
    P --> Q
    Q --> R{具备访问权限且Token充足}
    R -- 否 --> L
    R -- 是 --> S[流式回答与来源溯源]
    S --> T[记录消息用量和审计]
    T --> U([结束])
```

## 3. 文件上传与索引流程图

图3：文件版本化索引流程。只有新版本完整写入后，系统才替换当前检索版本。

```mermaid
flowchart TD
    A([收到上传或重新索引请求]) --> B{校验权限格式大小容量}
    B -- 否 --> C[返回结构化错误] --> Z([结束])
    B -- 是 --> D[保存原文件或新文本版本]
    D --> E[创建排队任务]
    E --> F[Worker领取任务]
    F --> G[解析与标准化文本]
    G --> H{解析成功}
    H -- 否 --> I[记录失败与重试次数]
    I --> J{仍可自动重试}
    J -- 是 --> E
    J -- 否 --> K[标记失败等待手动重试] --> Z
    H -- 是 --> L[切块并估算Embedding Token]
    L --> M{Token配额充足}
    M -- 否 --> K
    M -- 是 --> N[写入新版本向量]
    N --> O{全部写入成功}
    O -- 否 --> I
    O -- 是 --> P[事务切换Active Version]
    P --> Q[删除旧版本向量]
    Q --> R[更新统计用量与审计]
    R --> Z
```

## 4. 核心时序图

图4：Web 扫码登录时序图。登录票据短期有效且只能由已登录小程序确认一次。

```mermaid
sequenceDiagram
    autonumber
    actor 用户
    participant Web
    participant 小程序
    participant API
    participant PostgreSQL

    用户->>Web: 打开登录页
    Web->>API: POST /api/v1/auth/web/tickets
    API->>PostgreSQL: 创建5分钟票据
    PostgreSQL-->>API: 返回ticket_id
    API-->>Web: 返回二维码内容
    用户->>小程序: 扫描二维码
    小程序->>API: POST /tickets/{id}/confirm
    API->>PostgreSQL: 校验登录用户和票据状态
    alt 票据有效
        PostgreSQL-->>API: 原子更新为confirmed
        API-->>小程序: 确认成功
        Web->>API: GET /tickets/{id}
        API->>PostgreSQL: 读取确认结果并消费票据
        PostgreSQL-->>API: 返回用户与会话
        API-->>Web: 返回访问与刷新令牌
        Web-->>用户: 进入知识库
    else 票据过期或已消费
        PostgreSQL-->>API: 返回无效状态
        API-->>小程序: 返回TICKET_INVALID
        API-->>Web: 要求刷新二维码
    end
```

图5：流式问答与用量记录时序图。服务端先确定权限范围，再调用向量库和模型。

```mermaid
sequenceDiagram
    autonumber
    actor 用户
    participant 客户端
    participant API
    participant ChatService
    participant PostgreSQL
    participant VectorStore
    participant LLM

    用户->>客户端: 提交问题
    客户端->>API: POST /messages/stream
    API->>ChatService: 用户身份、会话、问题
    ChatService->>PostgreSQL: 校验库权限与Token额度
    alt 无权限或额度不足
        PostgreSQL-->>ChatService: 拒绝原因
        ChatService-->>API: 结构化错误
        API-->>客户端: SSE error + request_id
    else 校验通过
        PostgreSQL-->>ChatService: 可访问知识库范围
        ChatService->>VectorStore: 带范围过滤检索
        VectorStore-->>ChatService: 候选片段与元数据
        ChatService->>LLM: 问题与候选片段
        loop 流式生成
            LLM-->>ChatService: delta
            ChatService-->>API: delta
            API-->>客户端: SSE delta
        end
        LLM-->>ChatService: token usage
        ChatService->>PostgreSQL: 保存消息引用用量和审计
        PostgreSQL-->>ChatService: 提交成功
        ChatService-->>API: references + done
        API-->>客户端: SSE完成事件
    end
```

## 5. 技术架构图

图6：系统技术架构图。首版以单进程模块化单体满足单实例部署，同时为未来拆分 Worker 保留端口。

```mermaid
flowchart TB
    subgraph 客户端
        MP[微信原生小程序\n四页原型]
        WEB[Web前端\n原型同构]
    end

    subgraph 服务端
        subgraph 接入层
            GIN[Gin Router\nJWT与Request ID]
        end
        subgraph 业务层
            AUTH[Account\n微信与扫码登录]
            LIB[Library\n权限与公开访问]
            DOC[Document\n文件与内容版本]
            INDEX[Indexing Worker\n任务与版本切换]
            CHAT[Chat\n检索与SSE]
            USAGE[Usage\n配额与审计]
            SETTINGS[Settings\nRAG参数与会话安全]
        end
        subgraph 数据层
            PG[(PostgreSQL\n业务任务用量审计)]
            FILES[(本地文件目录\n原文件与文本版本)]
            VEC[(Mem或Milvus\n向量与过滤元数据)]
            LOGS[(JSON滚动日志\n30天)]
        end
    end

    subgraph 外部依赖
        WX[微信code2session]
        EMB[Embedding模型]
        MODEL[Chat LLM]
    end

    MP & WEB --> GIN
    GIN --> AUTH & LIB & DOC & CHAT & USAGE & SETTINGS
    INDEX --> PG & FILES & VEC
    AUTH & LIB & DOC & CHAT & USAGE & SETTINGS --> PG
    DOC --> FILES
    CHAT --> VEC
    AUTH --> WX
    INDEX --> EMB
    CHAT --> MODEL
    GIN & INDEX --> LOGS
```

## 6. 索引任务状态图

图7：索引任务状态机。失败任务最多自动重试三次，成功切换版本后进入终态。

```mermaid
stateDiagram-v2
    [*] --> 排队中 : 创建任务
    排队中 --> 处理中 : Worker领取
    处理中 --> 已就绪 : 写入完成并切换版本
    处理中 --> 排队中 : 可重试错误且未达上限
    处理中 --> 失败 : 不可重试或达到上限
    失败 --> 排队中 : 用户手动重试
    排队中 --> 已取消 : 文档删除或用户取消
    已就绪 --> [*]
    已取消 --> [*]
```

## 7. 文档删除状态图

图8：文档删除状态机。删除采用先标记后清理，避免数据库、文件和向量出现半删除状态。

```mermaid
stateDiagram-v2
    [*] --> 可用 : 索引完成
    可用 --> 删除中 : 所有者确认删除
    失败 --> 删除中 : 所有者确认删除
    删除中 --> 已删除 : 向量文件与业务记录清理完成
    删除中 --> 删除失败 : 清理失败
    删除失败 --> 删除中 : Worker重试
    已删除 --> [*]
```
