// AI assistant (Loggi). Which actions exist and their prompts are set by administrators.
import { reactive } from 'vue'
import { api } from './api'
import { mail, htmlToText, keyOf, type Compose, type FullMessage } from './mail'
import { longDate } from './format'
import { toast } from './toast'
import { t } from './i18n'

export type AIAction = { key: string, title: string, description: string }
export const ai = reactive({ actions: [] as AIAction[], loaded: false, busy: '' as string, summary: {} as Record<string, string | 'loading'> })

export async function loadAIActions() {
  try { ai.actions = await api<AIAction[]>('/api/ai/actions') } catch { ai.actions = [] }
  ai.loaded = true
}
export const hasAI = (key: string) => ai.actions.some(a => a.key === key)
// The assistant's name chosen by the administrator ("Loggi" by default); messages take it as {ai}.
export const aiName = () => mail.me?.aiName || 'Loggi'

// What the editor's right-click menu asks Loggi or the microphone for.
export type AssistKind = 'dictate' | 'dictateLoggi' | 'rephrase' | 'fix'
export type AIInput = { from?: string, to?: string, subject?: string, letter?: string, draft?: string, instructions?: string, mailbox?: string, templates?: string, attachments?: string }
export async function aiRun(key: string, input: AIInput & { thread?: string }): Promise<string> {
  const r = await api<{ text: string }>('/api/ai/run', { method: 'POST', body: { key, ...input }, timeout: 160000 })
  return plain(r.text)
}
// Letters are plain text: drop Markdown the model sometimes adds (**bold**, "* " bullets, headings).
export const plain = (s: string) => s.replace(/\*\*(.+?)\*\*/g, '$1').replace(/__(.+?)__/g, '$1').replace(/^\s*[*-]\s+/gm, '• ').replace(/^#{1,6}\s+/gm, '').trim()

// The letter being answered, as the model sees it.
export function letterInput(): AIInput {
  const m = mail.current
  if (!m) return {}
  return {
    from: m.from.name ? `${m.from.name} <${m.from.email}>` : m.from.email,
    to: m.to.map(a => a.email).join(', '), subject: m.subject, letter: htmlToText(m.html), mailbox: m.account,
  }
}

// ---------- the user's own part of a compose body (before signature / quote) ----------

const MARK = '[data-tmail-sig],[data-tmail-quote],[data-tmail-fwd]'
function split(html: string) {
  const doc = new DOMParser().parseFromString(`<body>${html}</body>`, 'text/html')
  const own: Node[] = []
  for (const n of Array.from(doc.body.childNodes)) {
    if (n instanceof Element && (n.matches(MARK) || n.querySelector(MARK))) break
    own.push(n)
  }
  return { doc, own }
}
export function composeUserText(c: Compose): string {
  const { doc, own } = split(c.html)
  const box = doc.createElement('div')
  own.forEach(n => box.append(n.cloneNode(true)))
  return htmlToText(box.innerHTML).trim()
}
const esc = (s: string) => s.replace(/[&<>"]/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[ch]!))
export function setComposeUserText(c: Compose, text: string) {
  const { doc, own } = split(c.html)
  const html = text.trim().replace(/\n{3,}/g, '\n\n').split('\n').map(l => `<div>${l.trim() ? esc(l) : '<br>'}</div>`).join('') + '<div><br></div>'
  const tmp = doc.createElement('div')
  tmp.innerHTML = html
  const first = own[0]
  if (first) first.parentNode!.insertBefore(tmp, first)
  else doc.body.prepend(tmp)
  own.forEach(n => n.parentNode?.removeChild(n))
  tmp.replaceWith(...Array.from(tmp.childNodes))
  c.html = doc.body.innerHTML
}

// ---------- whole conversation for the model ----------


const MAX_THREAD = 10
// Earlier letters of the open conversation, oldest first, as plain text.
export async function threadText(): Promise<string> {
  const cur = mail.current
  if (!cur) return ''
  const others = mail.thread.filter(x => keyOf(x) !== keyOf(cur) && (!cur.messageId || x.messageId !== cur.messageId)).slice(-MAX_THREAD)
  const parts = await Promise.all(others.map(async (x) => {
    const cached = mail.threadOpen[keyOf(x)]
    let full: FullMessage | null = typeof cached === 'object' ? cached : null
    if (!full) {
      try { full = await api<FullMessage>('/api/message', { query: { account: x.account, folder: x.folder, uid: x.uid, peek: 1 } }) } catch { full = null }
    }
    const who = x.from.name ? `${x.from.name} <${x.from.email}>` : x.from.email
    const body = full ? htmlToText(full.html).replace(/^>.*$/gm, '').replace(/\n{3,}/g, '\n\n').trim().slice(0, 3000) : x.preview
    return `--- ${longDate(x.date)}, ${who}:\n${body}`
  }))
  return parts.join('\n\n')
}

// Writes a reply into the composer: instructions may be typed or dictated.
export async function aiReplyInto(c: Compose, instructions: string) {
  if (ai.busy) return
  ai.busy = 'reply'
  const before = composeUserText(c)
  setComposeUserText(c, t('ai.busyReply', { ai: aiName() }))
  try {
    // Only the start template ("Hello,") is no draft: Loggi writes the greeting itself.
    const draft = before.replace(/\s+/g, ' ').trim() === c.template.replace(/\s+/g, ' ').trim() ? '' : before
    const input = { ...letterInput(), thread: await threadText(), draft, instructions }
    setComposeUserText(c, await aiRun('reply', input))
  } catch (e: any) {
    setComposeUserText(c, before)
    toast(e.message, { kind: 'err', ms: 8000 })
  } finally { ai.busy = '' }
}

// aiWriteInto: a new letter (not a reply) written by Loggi from the user's instructions.
export async function aiWriteInto(c: Compose, instructions: string) {
  if (ai.busy) return
  ai.busy = 'write'
  const before = composeUserText(c)
  const draft = before.replace(/\s+/g, ' ').trim() === c.template.replace(/\s+/g, ' ').trim() ? '' : before
  setComposeUserText(c, t('ai.busyWrite', { ai: aiName() }))
  try {
    setComposeUserText(c, await aiRun('write', { to: c.to, subject: c.subject, draft, instructions, mailbox: c.from }))
  } catch (e: any) {
    setComposeUserText(c, before)
    toast(e.message, { kind: 'err', ms: 8000 })
  } finally { ai.busy = '' }
}

export async function aiImproveInto(c: Compose, instructions: string) {
  const before = composeUserText(c)
  if (!before) { toast(t('ai.writeFirst', { ai: aiName() })); return }
  if (ai.busy) return
  ai.busy = 'improve'
  try { setComposeUserText(c, await aiRun('improve', { draft: before, instructions })) }
  catch (e: any) { toast(e.message, { kind: 'err', ms: 8000 }) }
  finally { ai.busy = '' }
}

export async function aiSummary() {
  const m = mail.current
  if (!m) return
  const k = keyOf(m)
  ai.summary[k] = 'loading'
  try { ai.summary[k] = await aiRun('summary', letterInput()) }
  catch (e: any) { delete ai.summary[k]; toast(e.message, { kind: 'err', ms: 8000 }) }
}

// Asks Loggi which reply template fits the open letter; returns the template id or 0.
export async function aiPickTemplate(list: { id: number, title: string, body: string }[]): Promise<number> {
  if (!list.length) return 0
  const templates = list.map((tp, i) => `${i + 1}. ${tp.title}: ${tp.body.replace(/\s+/g, ' ').slice(0, 300)}`).join('\n')
  try {
    const ans = await aiRun('pick_template', { ...letterInput(), templates } as any)
    const n = Number((ans.match(/\d+/) || ['0'])[0])
    return n >= 1 && n <= list.length ? list[n - 1].id : 0
  } catch (e: any) { toast(e.message, { kind: 'err' }); return 0 }
}

// ---------- "Reply with AI": questions of the letter → short answers of the operator → reply ----------

export type ReplyQuestion = { question: string, options: string[], answer: string }
export const replyPanel = reactive({
  open: false, loading: false, error: '', key: '',
  intent: '', questions: [] as ReplyQuestion[], clarify: [] as { text: string, on: boolean }[], extra: '',
})

// The model answers with a JSON object; take the outermost {...} in case it adds words around it.
function parseQuestions(raw: string) {
  const a = raw.indexOf('{'), b = raw.lastIndexOf('}')
  if (a < 0 || b <= a) throw new Error(t('ai.parseFailed', { ai: aiName() }))
  const j = JSON.parse(raw.slice(a, b + 1))
  const qs = (Array.isArray(j.questions) ? j.questions : []).slice(0, 6)
    .filter((q: any) => q && typeof q.question === 'string' && q.question.trim())
    .map((q: any) => ({ question: q.question.trim(), options: (Array.isArray(q.options) ? q.options : []).filter((o: any) => typeof o === 'string' && o.trim()).slice(0, 4), answer: '' }))
  const cl = (Array.isArray(j.clarify) ? j.clarify : []).filter((x: any) => typeof x === 'string' && x.trim()).slice(0, 4).map((x: string) => ({ text: x.trim(), on: false }))
  return { intent: typeof j.intent === 'string' ? j.intent.trim() : '', questions: qs as ReplyQuestion[], clarify: cl }
}

export async function openReplyPanel() {
  const m = mail.current
  if (!m) return
  Object.assign(replyPanel, { open: true, loading: true, error: '', key: keyOf(m), intent: '', questions: [], clarify: [], extra: '' })
  try {
    const text = await aiRun('reply_questions', { ...letterInput(), thread: await threadText(), attachments: m.attachments.map(a => a.name).join(', ') })
    if (replyPanel.key !== keyOf(m)) return // another letter was opened meanwhile
    Object.assign(replyPanel, parseQuestions(text))
  } catch (e: any) {
    replyPanel.error = e.message || t('ai.parseFailedGeneric')
  } finally { replyPanel.loading = false }
}

// What the operator decided, as instructions for the reply prompt.
export function replyInstructions(): string {
  const lines: string[] = []
  const qs = replyPanel.questions
  if (qs.length) {
    lines.push(t('ai.instrAnswers'))
    qs.forEach((q, i) => lines.push(`${i + 1}. ${q.question} — ${q.answer.trim() || t('ai.instrUnanswered')}`))
  }
  const cl = replyPanel.clarify.filter(c => c.on)
  if (cl.length) lines.push(t('ai.instrClarify', { items: cl.map(c => c.text).join('; ') }))
  if (replyPanel.extra.trim()) lines.push(t('ai.instrExtra', { text: replyPanel.extra.trim() }))
  return lines.join('\n')
}
