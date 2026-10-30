// Dictation: records the microphone in the browser and turns speech into text with Loggi ASR.
import { reactive } from 'vue'
import { toast } from './toast'
import { t } from './i18n'

export const voice = reactive({ recording: false, seconds: 0, busy: false, owner: '' })
let rec: MediaRecorder | null = null
let chunks: Blob[] = []
let timer: ReturnType<typeof setInterval> | null = null
let done: ((b: Blob | null) => void) | null = null
const MAX_SECONDS = 180

export async function startVoice(owner: string): Promise<boolean> {
  if (voice.recording || voice.busy) return false
  if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') { toast(t('voice.noRecorder'), { kind: 'err' }); return false }
  let stream: MediaStream
  try { stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } }) }
  catch { toast(t('voice.noMic'), { kind: 'err', ms: 8000 }); return false }
  const type = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/mp4'].find(mt => MediaRecorder.isTypeSupported(mt)) || ''
  rec = new MediaRecorder(stream, type ? { mimeType: type } : undefined)
  chunks = []
  rec.ondataavailable = e => { if (e.data.size) chunks.push(e.data) }
  rec.onstop = () => {
    stream.getTracks().forEach(tr => tr.stop())
    if (timer) clearInterval(timer)
    voice.recording = false
    const blob = chunks.length ? new Blob(chunks, { type: rec?.mimeType || 'audio/webm' }) : null
    done?.(blob)
    done = null
  }
  rec.start(250)
  Object.assign(voice, { recording: true, seconds: 0, owner })
  timer = setInterval(() => { if (++voice.seconds >= MAX_SECONDS) stopVoice() }, 1000)
  return true
}

// Stops and returns the recognised text ('' if cancelled or nothing heard).
export function stopVoice(cancel = false): Promise<string> {
  return new Promise((resolve) => {
    if (!rec || rec.state === 'inactive') { resolve(''); return }
    done = async (blob) => {
      if (cancel || !blob || blob.size < 1200) { resolve(''); return }
      voice.busy = true
      try {
        const fd = new FormData()
        fd.append('audio', blob, blob.type.includes('ogg') ? 'voice.ogg' : blob.type.includes('mp4') ? 'voice.m4a' : 'voice.webm')
        const r = await fetch('/api/ai/transcribe', { method: 'POST', body: fd, credentials: 'same-origin', headers: { 'X-ZM': '1' } })
        const j = await r.json().catch(() => ({}))
        if (!r.ok) throw new Error(j.error || t('voice.errorStatus', { status: r.status }))
        if (!j.text) toast(t('voice.notRecognized'), { kind: 'err' })
        resolve(j.text || '')
      } catch (e: any) {
        toast(e.message, { kind: 'err', ms: 8000 })
        resolve('')
      } finally { voice.busy = false }
    }
    rec.stop()
  })
}
