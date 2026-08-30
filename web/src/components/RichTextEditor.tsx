// wangeditor 受控封装：value(HTML)/onChange；卸载 destroy 防重复挂载与内存泄漏
import { useEffect, useRef, useState } from 'react'
import { Editor, Toolbar } from '@wangeditor/editor-for-react'
import type { IDomEditor, IEditorConfig, IToolbarConfig } from '@wangeditor/editor'
import '@wangeditor/editor/dist/css/style.css'

interface Props {
  value: string
  onChange: (html: string) => void
  height?: number
  placeholder?: string
}

// 工具栏裁剪：标题/加粗/列表/链接/图片（Demo 定位，图片 base64 入库）
const toolbarConfig: Partial<IToolbarConfig> = {
  excludeKeys: ['group-video', 'insertTable', 'codeBlock', 'todo', 'fullScreen'],
}

const RichTextEditor = ({ value, onChange, height = 300, placeholder = '请输入内容...' }: Props) => {
  const [editor, setEditor] = useState<IDomEditor | null>(null)
  const editorRef = useRef<IDomEditor | null>(null)

  // 组件卸载时销毁编辑器实例（React 18 StrictMode 双挂载下必做）
  useEffect(() => {
    editorRef.current = editor
    return () => {
      if (editorRef.current) {
        editorRef.current.destroy()
        editorRef.current = null
      }
    }
  }, [editor])

  const editorConfig: Partial<IEditorConfig> = {
    placeholder,
    readOnly: false,
    MENU_CONF: {
      uploadImage: {
        // Demo：base64 直接入库，不走上传接口
        base64LimitSize: 2 * 1024 * 1024,
        server: '',
      },
    },
  }

  return (
    <div style={{ border: '1px solid #d9d9d9', borderRadius: 6, zIndex: 100 }}>
      <Toolbar editor={editor} defaultConfig={toolbarConfig} mode="default" style={{ borderBottom: '1px solid #d9d9d9' }} />
      <Editor
        defaultConfig={editorConfig}
        value={value}
        onCreated={setEditor}
        onChange={(ed) => onChange(ed.getHtml())}
        mode="default"
        style={{ height, overflowY: 'hidden' }}
      />
    </div>
  )
}

export default RichTextEditor
