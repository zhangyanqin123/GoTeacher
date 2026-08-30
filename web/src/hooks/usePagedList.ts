// 搜索表格页统一分页 hook：翻页复用最近查询条件；requestId 竞态丢弃（快速翻页时旧响应作废）
import { useCallback, useRef, useState } from 'react'

import type { PageReq, PageResp } from '@/api/types'

interface Options {
  defaultPageSize?: number
}

export function usePagedList<T, Q extends object>(fetcher: (query: Q & PageReq) => Promise<PageResp<T>>, opts: Options = {}) {
  const [list, setList] = useState<T[]>([])
  const [count, setCount] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(opts.defaultPageSize ?? 10)

  const queryRef = useRef<Q | null>(null) // 最近一次查询条件（search 时更新，翻页/刷新复用）
  const pageRef = useRef(1)
  const pageSizeRef = useRef(opts.defaultPageSize ?? 10)
  const requestId = useRef(0)

  const doFetch = useCallback(
    async (query: Q, pageIndex: number, size: number) => {
      const id = ++requestId.current
      setLoading(true)
      try {
        const resp = await fetcher({ ...query, page_index: pageIndex, page_size: size })
        if (id !== requestId.current) return // 过期响应丢弃
        setList(resp.list ?? [])
        setCount(resp.count ?? 0)
      } finally {
        if (id === requestId.current) setLoading(false)
      }
    },
    [fetcher],
  )

  // 查询按钮：回第 1 页
  const search = useCallback(
    (getQuery: () => Q) => {
      const query = getQuery()
      queryRef.current = query
      pageRef.current = 1
      setPage(1)
      void doFetch(query, 1, pageSizeRef.current)
    },
    [doFetch],
  )

  // 重置：空条件回第 1 页
  const reset = useCallback(
    (emptyQuery: Q) => {
      search(() => emptyQuery)
    },
    [search],
  )

  // 增删改后保持当前页重发
  const reload = useCallback(() => {
    if (queryRef.current) void doFetch(queryRef.current, pageRef.current, pageSizeRef.current)
  }, [doFetch])

  const onPaginationChange = useCallback(
    (nextPage: number, nextSize: number) => {
      pageRef.current = nextPage
      pageSizeRef.current = nextSize
      setPage(nextPage)
      setPageSize(nextSize)
      if (queryRef.current) void doFetch(queryRef.current, nextPage, nextSize)
    },
    [doFetch],
  )

  return { list, count, loading, page, pageSize, search, reset, reload, onPaginationChange }
}
