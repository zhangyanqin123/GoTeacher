// 老师下拉选项（/options，含停用）：模块级缓存，编辑/离职弹窗共用
import { useEffect, useState } from 'react'

import { listTeacherOptions, type TeacherOption } from '@/api/teacher'

let cache: TeacherOption[] | null = null
let pending: Promise<TeacherOption[]> | null = null

export const loadTeacherOptions = async (force = false): Promise<TeacherOption[]> => {
  if (cache && !force) return cache
  if (!pending || force) pending = listTeacherOptions().then((list) => { cache = list; return list })
  return pending
}

export const useTeacherOptions = () => {
  const [options, setOptions] = useState<TeacherOption[]>(cache ?? [])
  const [loading, setLoading] = useState(cache === null)

  useEffect(() => {
    if (cache) return
    let alive = true
    loadTeacherOptions()
      .then((list) => { if (alive) setOptions(list) })
      .catch(() => undefined)
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [])

  return { options, loading }
}
