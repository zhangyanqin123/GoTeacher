// 富文本只读展示：DOMPurify 净化后渲染（全项目 dangerouslySetInnerHTML 唯一收口）
// 后端 bluemonday 是存储侧主防线，此处是展示侧二次净化（防御纵深）
import DOMPurify from 'dompurify'

const RichTextView = ({ html, minHeight = 60 }: { html: string; minHeight?: number }) => {
  if (!html || html.replace(/<[^>]*>/g, '').trim() === '') return <span style={{ color: '#999' }}>-</span>
  return (
    <div
      className="rich-view"
      style={{ minHeight, lineHeight: 1.7, wordBreak: 'break-word' }}
      dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(html) }}
    />
  )
}

export default RichTextView
