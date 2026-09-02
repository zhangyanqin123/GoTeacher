-- ============================================================
-- AB 版模块配置种子（C 端 H5 gyz-h5-spacestation 首页模块显隐首版清单）：
-- spacestation 空间站 / f10 各 8 个配置项，取自 H5 src/config/abModules.ts
-- （carouselAd 广告轮播位仅数据版可见，其余两版均显）。
-- 模块显式写死 id 1/2，item 的 module_id 引用固定——空表首插 AUTO_INCREMENT
-- 必从 1 起，确定性成立（diagnose seed 显式 id 同模式）。
-- 判空只看 ab_module（seedAbModule 单函数管两表，见 database.go）
-- ============================================================

INSERT INTO ab_module (id, module_key, module_name, sort_no) VALUES
  (1, 'spacestation', '空间站', 1),
  (2, 'f10', 'F10', 2);

INSERT INTO ab_module_item (module_id, item_key, item_name, versions, sort_no) VALUES
  (1, 'topBanner',  '顶部图',                            'mass,data', 1),
  (1, 'tabModule',  'tab 跳转',                          'mass,data', 2),
  (1, 'carouselAd', '广告轮播位',                        'data',      3),
  (1, 'hotRank',    '热选赛道榜',                        'mass,data', 4),
  (1, 'promo',      '模方冲锋榜',                        'mass,data', 5),
  (1, 'hotCard',    '模方金股榜',                        'mass,data', 6),
  (1, 'recommend',  '短线胜率/涨停潜伏/短线收益 TOP10',   'mass,data', 7),
  (1, 'disclaimer', '免责申明',                          'mass,data', 8),
  (2, 'topBanner',  '顶部图',   'mass,data', 1),
  (2, 'tabModule',  'tab 跳转', 'mass,data', 2),
  (2, 'carouselAd', '广告轮播位', 'data',      3),
  (2, 'hotRank',    '热选赛道榜', 'mass,data', 4),
  (2, 'promo',      '模方冲锋榜', 'mass,data', 5),
  (2, 'hotCard',    '模方金股榜', 'mass,data', 6),
  (2, 'recommend',  '短线胜率/涨停潜伏/短线收益 TOP10', 'mass,data', 7),
  (2, 'disclaimer', '免责申明',  'mass,data', 8);
