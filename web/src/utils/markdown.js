// markdown.js —— 极简 Markdown 渲染
//
// 为什么不直接上 marked / markdown-it？
//   1. 答案来自大模型，属于"不可信文本"，自己写反而更容易保证转义到位；
//   2. 这里只需要笔记和回答里真正常用的那几种语法，几十行就够；
//   3. 少一个运行时依赖，构建产物也更小。
//
// 安全前提：**先整体转义，再做替换**。所有输出到 HTML 的尖括号都已经是 &lt; 形态，
// 因此模型即使吐出 <img onerror=...> 也只会被当纯文本显示。

const ESCAPES = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

function escapeHtml(text) {
  return text.replace(/[&<>"']/g, (ch) => ESCAPES[ch])
}

// 行内语法：`代码`、**加粗**
function inline(text) {
  return text
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
}

// renderMarkdown 把 Markdown 文本转成安全的 HTML 字符串
//
// 支持：``` 代码块、#~#### 标题、-/*/1. 列表、> 引用、行内代码、加粗。
// 特意容忍"未闭合的代码块"——流式输出时围栏还没等到后半截，
// 这时候也要把内容按代码渲染出来，否则画面会来回跳。
export function renderMarkdown(source = '') {
  const lines = escapeHtml(source).split('\n')
  const out = []
  let inCode = false
  let inList = false

  const closeList = () => {
    if (inList) {
      out.push('</ul>')
      inList = false
    }
  }

  for (const line of lines) {
    if (/^\s*```/.test(line)) {
      closeList()
      out.push(inCode ? '</code></pre>' : '<pre class="md-pre"><code>')
      inCode = !inCode
      continue
    }

    if (inCode) {
      out.push(line)
      continue
    }

    if (!line.trim()) {
      closeList()
      continue
    }

    const heading = line.match(/^(#{1,4})\s+(.*)$/)
    if (heading) {
      closeList()
      // 页面上已有 h1/h2，这里从 h3 起，避免层级倒挂
      const level = Math.min(heading[1].length + 2, 6)
      out.push(`<h${level}>${inline(heading[2])}</h${level}>`)
      continue
    }

    const item = line.match(/^\s*(?:[-*+]|\d+\.)\s+(.*)$/)
    if (item) {
      if (!inList) {
        out.push('<ul>')
        inList = true
      }
      out.push(`<li>${inline(item[1])}</li>`)
      continue
    }

    const quote = line.match(/^&gt;\s?(.*)$/) // '>' 已被转义成 &gt;
    if (quote) {
      closeList()
      out.push(`<blockquote>${inline(quote[1])}</blockquote>`)
      continue
    }

    closeList()
    out.push(`<p>${inline(line)}</p>`)
  }

  closeList()
  if (inCode) out.push('</code></pre>') // 流式过程中围栏可能还没闭合
  return out.join('\n')
}
