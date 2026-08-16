-- 种子数据：接口样例值（000001/today）
-- stat_date 用 CURDATE()，保证"今天"这个区间始终能命中
-- 注意：样例 total=2312 与 9 档之和 2309 不一致，照抄原样（README 有说明）
INSERT INTO house_up_down_stats
  (secu_market, stat_range, above7, between5_7, between3_5, between0_3, equal0,
   between_n3_0, between_n5_n3, between_n7_n5, below_n7,
   total, up_count, down_count, flat_count, stat_date)
VALUES
  ('000001', 'today', 53, 37, 111, 1352, 87, 635, 30, 3, 1, 2312, 1553, 669, 87, CURDATE());