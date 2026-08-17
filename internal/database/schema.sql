-- 涨跌家数分布统计表
-- range 是 MySQL 保留字，列名用 stat_range，Go 侧字段仍叫 Range（靠 db tag 映射）
CREATE TABLE IF NOT EXISTS house_up_down_stats (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  secu_market   VARCHAR(10)     NOT NULL                COMMENT '市场代码，如 000001',
  stat_range    VARCHAR(10)     NOT NULL                COMMENT '统计区间：today/week/month',
  above7        INT             NOT NULL DEFAULT 0      COMMENT '涨幅 >7% 家数',
  between5_7    INT             NOT NULL DEFAULT 0      COMMENT '涨幅 5%~7% 家数',
  between3_5    INT             NOT NULL DEFAULT 0      COMMENT '涨幅 3%~5% 家数',
  between0_3    INT             NOT NULL DEFAULT 0      COMMENT '涨幅 0~3% 家数',
  equal0        INT             NOT NULL DEFAULT 0      COMMENT '平盘家数',
  between_n3_0  INT             NOT NULL DEFAULT 0      COMMENT '跌幅 0~-3% 家数',
  between_n5_n3 INT             NOT NULL DEFAULT 0      COMMENT '跌幅 -3%~-5% 家数',
  between_n7_n5 INT             NOT NULL DEFAULT 0      COMMENT '跌幅 -5%~-7% 家数',
  below_n7      INT             NOT NULL DEFAULT 0      COMMENT '跌幅 >7% 家数',
  total         INT             NOT NULL DEFAULT 0      COMMENT '总家数',
  up_count      INT             NOT NULL DEFAULT 0      COMMENT '上涨家数',
  down_count    INT             NOT NULL DEFAULT 0      COMMENT '下跌家数',
  flat_count    INT             NOT NULL DEFAULT 0      COMMENT '平盘家数',
  stat_date     DATE            NOT NULL                COMMENT '统计日期',
  created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_market_range_date (secu_market, stat_range, stat_date),
  KEY idx_stat_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='涨跌家数分布统计';

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
--   transfer_content 存逗号分隔 'group,friend'，接口输出为数组
--   salesman_name/dept 存原老师全部绑定业务员，多个逗号分隔
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
  transfer_content         VARCHAR(32)     NOT NULL DEFAULT ''     COMMENT '转移内容：group 转移客户群 / friend 转移好友，逗号分隔',
  group_count              INT             NOT NULL DEFAULT 0      COMMENT '转移客户群数',
  friend_count             INT             NOT NULL DEFAULT 0      COMMENT '转移好友数',
  operator                 VARCHAR(50)     NOT NULL DEFAULT ''     COMMENT '操作人（无登录态固定 admin，对齐 teacher.update_by）',
  operate_ip               VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '操作IP（handler 取 c.ClientIP()）',
  transfer_time            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '转移时间（INSERT 时 NOW()）',
  remark                   VARCHAR(200)    NOT NULL DEFAULT ''     COMMENT '备注',
  created_at               DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at               DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  KEY idx_original_dept (original_teacher_dept_id),
  KEY idx_original_teacher (original_teacher_id),
  KEY idx_replace_teacher (replace_teacher_id),
  KEY idx_transfer_time (transfer_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='老师离职转移记录';