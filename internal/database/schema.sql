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