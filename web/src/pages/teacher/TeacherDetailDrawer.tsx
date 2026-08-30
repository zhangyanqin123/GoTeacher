// 老师详情 Drawer：行数据全字段展示（编辑弹窗走 detail 接口，此处直接展示列表行冗余）
import { Drawer, Descriptions, Tag } from 'antd'

import type { TeacherRow } from '@/api/teacher'
import { TEACHER_LEVEL, TEACHER_STATUS } from '@/constants/teacher'
import StatusTag from '@/components/StatusTag'
import { text } from '@/utils/format'

const TeacherDetailDrawer = ({ row, onClose }: { row: TeacherRow | null; onClose: () => void }) => (
  <Drawer title={`老师详情（ID: ${row?.id ?? '-'}）`} open={row !== null} onClose={onClose} width={520}>
    {row && (
      <Descriptions column={1} bordered size="small">
        <Descriptions.Item label="老师账号">{row.account}</Descriptions.Item>
        <Descriptions.Item label="老师姓名">{text(row.name)}</Descriptions.Item>
        <Descriptions.Item label="老师昵称">{text(row.nickname)}</Descriptions.Item>
        <Descriptions.Item label="老师头衔">{text(row.title)}</Descriptions.Item>
        <Descriptions.Item label="执业资质">{row.qualification ? <Tag color={row.qualification === '已认证' ? 'success' : 'default'}>{row.qualification}</Tag> : '-'}</Descriptions.Item>
        <Descriptions.Item label="部门">{text(row.dept_name)}（ID: {row.dept_id}）</Descriptions.Item>
        <Descriptions.Item label="手机号">{text(row.phone)}</Descriptions.Item>
        <Descriptions.Item label="工号">{text(row.work_no)}</Descriptions.Item>
        <Descriptions.Item label="评级">{TEACHER_LEVEL.find((d) => d.value === row.rating)?.label ?? row.rating}</Descriptions.Item>
        <Descriptions.Item label="账号状态"><StatusTag dict={TEACHER_STATUS} value={row.status} /></Descriptions.Item>
        <Descriptions.Item label="绑定业务员数">{row.bind_sales_count}</Descriptions.Item>
        <Descriptions.Item label="头像">{row.avatar ? <a href={row.avatar} target="_blank" rel="noreferrer">{row.avatar}</a> : '-'}</Descriptions.Item>
        <Descriptions.Item label="个性签名">{text(row.signature)}</Descriptions.Item>
        <Descriptions.Item label="创建时间">{text(row.created_at)}</Descriptions.Item>
        <Descriptions.Item label="更新时间">{text(row.updated_at)}</Descriptions.Item>
        <Descriptions.Item label="更新人">{text(row.update_by)}</Descriptions.Item>
      </Descriptions>
    )}
  </Drawer>
)

export default TeacherDetailDrawer
