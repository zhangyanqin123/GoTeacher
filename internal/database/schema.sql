-- ============================================================
-- 老师管理（chatSys）
-- 对应前端 gyz-admin teacher.js / teacherQuery.vue
-- 兼容约定（前端直接比较，勿改）：
--   qualification 存中文 '已认证'/'未认证'
--   status 库里存 TINYINT 1/0，接口输出字符串 "1"/"0"
--   rating 存原值（种子 1-5 星，编辑后 0 无/1 初级/2 高级）
-- ============================================================

-- 老师
CREATE TABLE IF NOT EXISTS teacher (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  account       VARCHAR(50)     NOT NULL                COMMENT '账号',
  name          VARCHAR(50)     NOT NULL                COMMENT '姓名',
  nickname      VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '昵称',
  title         VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '头衔',
  qualification VARCHAR(8)      NOT NULL DEFAULT '未认证' COMMENT '执业资质：已认证/未认证',
  dept_id       BIGINT          NOT NULL DEFAULT 0       COMMENT '部门ID',
  dept_name     VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '部门名（冗余，与 mock 同构）',
  phone         VARCHAR(20)     NOT NULL DEFAULT ''      COMMENT '手机号',
  work_no       VARCHAR(20)     NOT NULL DEFAULT ''      COMMENT '工号',
  status        TINYINT         NOT NULL DEFAULT 1       COMMENT '账号状态：1 启用 / 0 停用',
  rating        INT             NOT NULL DEFAULT 0       COMMENT '评级（种子为 1-5，编辑后 0 无/1 初级/2 高级）',
  avatar        TEXT                    DEFAULT NULL     COMMENT '头像（base64 data URL）',
  signature     VARCHAR(200)    NOT NULL DEFAULT ''      COMMENT '个性签名',
  update_by     VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '更新人',
  created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  KEY idx_dept_id (dept_id),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='老师';

-- 业务员（桩表：真实系统为 admin 的 sys_user，种子 = mock 的 salesPool）
CREATE TABLE IF NOT EXISTS sales_user (
  id        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键（对齐前端 userIds）',
  phone     VARCHAR(20)  NOT NULL                COMMENT '手机号',
  nickname  VARCHAR(50)  NOT NULL                COMMENT '昵称',
  dept_name VARCHAR(50)  NOT NULL DEFAULT ''      COMMENT '部门名',
  PRIMARY KEY (id),
  UNIQUE KEY uk_phone (phone)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='业务员';

-- 老师-业务员绑定关系（全量替换语义：bind 时先删后插）
CREATE TABLE IF NOT EXISTS teacher_sales (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  teacher_id BIGINT UNSIGNED NOT NULL                COMMENT '老师ID',
  user_id    BIGINT UNSIGNED NOT NULL                COMMENT '业务员ID（sales_user.id）',
  bind_time  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '绑定时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_teacher_user (teacher_id, user_id),
  KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='老师-业务员绑定';

-- ============================================================
-- 诊股记录（对应前端 diagnoseSys/diagnose.js / diagnoseQuery.vue）
-- 兼容约定（与 mock 同构，勿改）：
--   昵称/姓名/股票名/老师为冗余快照（用户/老师可能被改/删，记录保留当时值）
--   buy_price DECIMAL(10,2)，接口输出浮点（1680.5）
--   report_content 富文本 HTML，未编写为空串（TEXT NOT NULL 避免 NULL 扫 string）
--   report_submit_time 可空，NULL=未提交（接口输出空串）
--   status：1 待诊股 / 2 待专业审核 / 3 专业审核不通过
--           4 待合规审核 / 5 合规审核不通过 / 6 合规审核通过
--   模糊查询列为前导通配 LIKE，不建索引（打不进 B-tree）
-- ============================================================

-- 诊股记录
CREATE TABLE IF NOT EXISTS diagnose (
  id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  user_nick_name     VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '用户昵称（冗余快照）',
  user_name          VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '用户姓名（冗余快照）',
  stock_code         VARCHAR(20)     NOT NULL DEFAULT ''      COMMENT '股票代码，如 SH600519',
  stock_name         VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '股票名称（冗余快照）',
  buy_price          DECIMAL(10,2)   NOT NULL DEFAULT 0.00    COMMENT '买入价格（元，两位小数）',
  buy_num            INT UNSIGNED    NOT NULL DEFAULT 0       COMMENT '买入股数',
  teacher_name       VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '诊股老师（冗余快照）',
  submit_time        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '用户提交时间',
  report_content     TEXT            NOT NULL                 COMMENT '诊股报告（富文本 HTML，未编写为空串）',
  report_submit_time DATETIME                 DEFAULT NULL    COMMENT '诊股提审时间（NULL=未提交）',
  status             TINYINT         NOT NULL DEFAULT 1       COMMENT '状态：1待诊股 2待专业审核 3专业审核不通过 4待合规审核 5合规审核不通过 6合规审核通过',
  remark             TEXT            NOT NULL                COMMENT '用户备注（富文本 HTML，净化后存储）',
  created_at         DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at         DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  KEY idx_status (status),
  KEY idx_submit_time (submit_time),
  KEY idx_report_submit_time (report_submit_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='诊股记录';

-- 诊股审核流程日志（落库为准，保留驳回历史与驳回原因；详情弹窗按 id 正序展示）
CREATE TABLE IF NOT EXISTS diagnose_audit_log (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  diagnose_id BIGINT UNSIGNED NOT NULL                COMMENT '诊股记录ID（diagnose.id）',
  log_type    VARCHAR(10)     NOT NULL                COMMENT '类型：用户提交/诊股报告/专业审核/合规审核',
  operator    VARCHAR(50)     NOT NULL DEFAULT ''      COMMENT '操作人：用户姓名/诊股老师/专业审核员/合规审核员',
  result      VARCHAR(8)      NOT NULL                COMMENT '结果：已提交/通过/不通过',
  remark      TEXT            NOT NULL                COMMENT '备注：用户remark / 固定文案 / 驳回原因（HTML）',
  log_time    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '记录时间',
  created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (id),
  KEY idx_diagnose_id (diagnose_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='诊股审核流程日志';

-- ============================================================
-- 老师离职转移（chatSys，对应前端 resign.js / teacherQuery.vue 离职转移 Tab）
-- 兼容约定（与 mock 同构，勿改）：
--   姓名/部门为冗余快照（离职后老师可能被改/删，记录保留转移当时值）
--   original_teacher_dept_id 供 deptId 筛选（mock 按 originalTeacherDeptId 过滤）
--   salesman_name/dept 存原老师全部绑定业务员，多个逗号分隔
--   transfer_content 为转移内容自由文本（2026-08-19 由 remark 改名，如「首席投顾」）
-- ============================================================

-- 老师离职转移记录
CREATE TABLE IF NOT EXISTS teacher_resign (
  id                       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  original_teacher_id      BIGINT UNSIGNED NOT NULL                COMMENT '原老师ID（teacher.id）',
  original_teacher_name    VARCHAR(50)     NOT NULL DEFAULT ''     COMMENT '原老师姓名（冗余快照）',
  original_teacher_dept_id BIGINT          NOT NULL DEFAULT 0      COMMENT '原老师部门ID（冗余，deptId 筛选用）',
  original_teacher_dept    VARCHAR(50)     NOT NULL DEFAULT ''     COMMENT '原老师部门名（冗余快照）',
  replace_teacher_id       BIGINT UNSIGNED NOT NULL                COMMENT '接替老师ID（teacher.id）',
  replace_teacher_name     VARCHAR(50)     NOT NULL DEFAULT ''     COMMENT '接替老师姓名（冗余快照）',
  replace_teacher_dept     VARCHAR(50)     NOT NULL DEFAULT ''     COMMENT '接替老师部门名（冗余快照）',
  salesman_name            VARCHAR(500)    NOT NULL DEFAULT ''     COMMENT '业务员姓名（原老师全部绑定业务员，逗号分隔）',
  salesman_dept            VARCHAR(500)    NOT NULL DEFAULT ''     COMMENT '业务员部门（逗号分隔，与姓名一一对应）',
  group_count              INT             NOT NULL DEFAULT 0      COMMENT '转移客户群数（=原老师绑定业务员数，后端计算入库）',
  operator                 VARCHAR(50)     NOT NULL DEFAULT ''     COMMENT '操作人（无登录态固定 admin，对齐 teacher.update_by）',
  operate_ip               VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '操作IP（handler 取 c.ClientIP()）',
  transfer_time            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '转移时间（INSERT 时 NOW()）',
  transfer_content         VARCHAR(200)    NOT NULL DEFAULT ''     COMMENT '转移内容（自由文本，如：首席投顾；原 remark 列改名）',
  created_at               DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at               DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  KEY idx_original_dept (original_teacher_dept_id),
  KEY idx_original_teacher (original_teacher_id),
  KEY idx_replace_teacher (replace_teacher_id),
  KEY idx_transfer_time (transfer_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='老师离职转移记录';
-- ============================================================
-- 管理员账号（鉴权，见 PLAN-auth.md）
-- 密码列存 bcrypt 哈希（定长 60）；种子由 Go 代码动态生成
-- （bcrypt 哈希无法手写 SQL，且固定哈希串入库等于泄露口令）
-- ============================================================

CREATE TABLE IF NOT EXISTS admin_user (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  username      VARCHAR(50)  NOT NULL                   COMMENT '登录账号',
  password      CHAR(60)     NOT NULL                   COMMENT '密码（bcrypt 哈希，定长 60）',
  nickname      VARCHAR(50)  NOT NULL DEFAULT ''        COMMENT '显示名（getinfo 的 name）',
  role          VARCHAR(50)  NOT NULL DEFAULT 'admin'   COMMENT '角色（getinfo 的 roles 单元素）',
  avatar        VARCHAR(255) NOT NULL DEFAULT ''        COMMENT '头像（空串，前端自行拼前缀）',
  status        TINYINT      NOT NULL DEFAULT 1         COMMENT '1 启用 / 0 停用',
  last_login_at DATETIME                DEFAULT NULL    COMMENT '最近登录时间',
  last_login_ip VARCHAR(64)  NOT NULL DEFAULT ''        COMMENT '最近登录 IP',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='管理员账号';

-- ============================================================
-- 订单系统 Demo（Gin → MySQL → RabbitMQ 异步链路，见 PLAN-order.md）
-- 兼容约定（与前端 gyz-admin orderDemo 页面同构）：
--   amount DECIMAL(10,2)，接口输出浮点（对齐 diagnose.buy_price）
--   status：1 处理中 / 2 已完成 / 3 已取消（库存不足）
--   stock_status/points_status/notify_status：0 待处理 / 1 成功 / 2 失败，
--     分别由 order.stock/order.points/order.notify 三个消费者回写，
--     三列全 1 时最后一个回写者将 status 置 2（条件 UPDATE，无竞态）
--   points_record.order_id / notification.order_id 唯一键 = MQ 消息幂等：
--     fanout 重投时 INSERT IGNORE 不重复落库
-- ============================================================

-- 商品（含库存）
CREATE TABLE IF NOT EXISTS product (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  product_name VARCHAR(100)    NOT NULL                COMMENT '商品名称',
  price        DECIMAL(10,2)   NOT NULL DEFAULT 0.00   COMMENT '单价（元，两位小数）',
  stock        INT UNSIGNED    NOT NULL DEFAULT 0      COMMENT '库存',
  created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='商品';

-- 订单（order 是 SQL 保留字，表名用复数）
CREATE TABLE IF NOT EXISTS orders (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  order_no      VARCHAR(32)     NOT NULL                COMMENT '订单号（时间戳+随机后缀，后端生成）',
  user_id       BIGINT UNSIGNED NOT NULL                COMMENT '下单用户ID（admin_user.id，取登录态）',
  product_id    BIGINT UNSIGNED NOT NULL                COMMENT '商品ID（product.id）',
  product_name  VARCHAR(100)    NOT NULL DEFAULT ''     COMMENT '商品名称（冗余快照，商品可能被改/删，记录保留当时值）',
  quantity      INT UNSIGNED    NOT NULL DEFAULT 1      COMMENT '购买数量',
  amount        DECIMAL(10,2)   NOT NULL DEFAULT 0.00   COMMENT '订单金额（price*quantity，后端计算）',
  status        TINYINT         NOT NULL DEFAULT 1      COMMENT '状态：1处理中 2已完成 3已取消（库存不足）',
  stock_status  TINYINT         NOT NULL DEFAULT 0      COMMENT '扣库存：0待处理 1成功 2失败（order.stock 消费者回写）',
  points_status TINYINT         NOT NULL DEFAULT 0      COMMENT '加积分：0待处理 1成功 2失败（order.points 消费者回写）',
  notify_status TINYINT         NOT NULL DEFAULT 0      COMMENT '发通知：0待处理 1成功 2失败（order.notify 消费者回写）',
  created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_order_no (order_no),
  KEY idx_user (user_id),
  KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='订单';

-- 积分流水（order_id 唯一键 = MQ 消息幂等：重投不重复加分）
CREATE TABLE IF NOT EXISTS points_record (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  user_id    BIGINT UNSIGNED NOT NULL                COMMENT '用户ID（订单下单人）',
  order_id   BIGINT UNSIGNED NOT NULL                COMMENT '订单ID（唯一，幂等键）',
  order_no   VARCHAR(32)     NOT NULL DEFAULT ''     COMMENT '订单号（冗余快照）',
  points     INT             NOT NULL DEFAULT 0      COMMENT '积分（1 元 1 分，按订单金额向下取整）',
  remark     VARCHAR(200)    NOT NULL DEFAULT ''     COMMENT '备注',
  created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_order (order_id),
  KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='积分流水';

-- 通知记录（order_id 唯一键 = MQ 消息幂等：重投不重复发通知）
CREATE TABLE IF NOT EXISTS notification (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  user_id    BIGINT UNSIGNED NOT NULL                COMMENT '用户ID（订单下单人）',
  order_id   BIGINT UNSIGNED NOT NULL                COMMENT '订单ID（唯一，幂等键）',
  title      VARCHAR(100)    NOT NULL DEFAULT ''     COMMENT '标题',
  content    VARCHAR(500)    NOT NULL DEFAULT ''     COMMENT '内容',
  is_read    TINYINT         NOT NULL DEFAULT 0      COMMENT '已读：1 是 / 0 否',
  created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_order (order_id),
  KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='通知记录';

-- ============================================================
-- AB 版模块配置（C 端 H5 gyz-h5-spacestation 各页面模块显隐，见 PLAN-ab-module.md）
-- 两级结构：模块（页面域，如 spacestation 空间站 / f10）→ 配置项（模块内 UI 块）
-- 兼容约定：
--   module_key / item_key 是 H5 代码引用的业务标识，创建后不可改（编辑接口不含此字段）
--   item_key 的值是 camelCase 原文（topBanner），是业务数据不是接口字段名，
--     不受本项目 JSON 键名 snake_case 约束
--   versions 逗号分隔枚举串（'mass,data'）：mass 大众版 / data 数据版，
--     值域固定枚举、每项不含逗号，无 JSON 列需求；service 层 join/split + 值域校验
--   item 的 module_id 是逻辑外键（不建 FOREIGN KEY，同项目其他表先例）
-- ============================================================

-- AB 版模块（父级）
CREATE TABLE IF NOT EXISTS ab_module (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  module_key  VARCHAR(50)     NOT NULL                COMMENT '模块标识（H5 页面域，聚合接口返回 map 第一级 key，如 spacestation；创建后不可改）',
  module_name VARCHAR(100)    NOT NULL DEFAULT ''     COMMENT '模块名称（管理台展示，如 空间站）',
  sort_no     INT             NOT NULL DEFAULT 0      COMMENT '排序号（管理台列表升序）',
  created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_module_key (module_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AB 版模块';

-- AB 版模块配置项（子级）；uk_module_item 一键两用：同模块内 item_key 唯一（跨模块允许同名）+ 最左前缀服务按模块查/数子项
CREATE TABLE IF NOT EXISTS ab_module_item (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  module_id  BIGINT UNSIGNED NOT NULL                COMMENT '所属模块 ID（ab_module.id，逻辑外键）',
  item_key   VARCHAR(50)     NOT NULL                COMMENT '配置项标识（H5 代码 camelCase key 原文，如 topBanner；业务数据不受 snake_case 约束，创建后不可改）',
  item_name  VARCHAR(100)    NOT NULL DEFAULT ''     COMMENT '配置项名称（管理台展示，如 顶部图）',
  versions   VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '可见版本逗号分隔串：mass 大众版 / data 数据版，如 mass,data（至少一项）',
  sort_no    INT             NOT NULL DEFAULT 0      COMMENT '排序号（管理台列表升序）',
  created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_module_item (module_id, item_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='AB 版模块配置项';
