<script setup>
import QRCode from 'qrcode'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { completeLogin, notify } from '../store'
import { pollLoginTicket } from '../utils/auth'

const router = useRouter()
const qrImage = ref('')
const status = ref('loading')
const error = ref('')
let controller = null

async function start() {
  controller?.abort()
  controller = new AbortController()
  qrImage.value = ''
  status.value = 'loading'
  error.value = ''
  try {
    const result = await pollLoginTicket({
      createTicket: api.createTicket,
      pollTicket: api.pollTicket,
      signal: controller.signal,
      onTicket: async (ticket) => { qrImage.value = await QRCode.toDataURL(ticket.qr_payload, { width: 280, margin: 1 }) },
      onState: (value) => { status.value = value },
    })
    await completeLogin(result)
    notify('登录成功，欢迎回来')
    router.replace('/ask')
  } catch (caught) {
    if (caught.name === 'AbortError') return
    status.value = caught.code === 'TICKET_INVALID' ? 'expired' : 'error'
    error.value = caught.message
  }
}

onMounted(start)
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <main class="login-page">
    <section class="login-story">
      <span class="login-brand">智深心流 · PERSON KNOW</span>
      <h1>让每一份资料，<br><em>都能回答你的问题。</em></h1>
      <p>个人知识库把原文件、语义检索、可信引用与审计记录收进一个专属空间。</p>
      <div class="login-orbit"><i /><i /><i /><strong>⌘</strong></div>
    </section>
    <section class="login-card">
      <header><span>微信安全登录</span><small>WEB ACCESS</small></header>
      <div class="qr-shell" :class="{ muted: status === 'expired' || status === 'error' }">
        <img v-if="qrImage" :src="qrImage" alt="微信扫码登录二维码">
        <span v-else class="qr-loading">正在生成二维码…</span>
      </div>
      <h2>{{ status === 'consumed' ? '登录成功' : status === 'expired' ? '二维码已过期' : '使用小程序扫码确认' }}</h2>
      <p v-if="error" class="login-error">{{ error }}</p>
      <p v-else>打开个人知识库小程序，扫描二维码并确认本次 Web 登录。</p>
      <button v-if="status === 'expired' || status === 'error'" type="button" @click="start">刷新二维码</button>
      <footer><span class="online-dot" /> 加密票据 · 五分钟有效 · 单次消费</footer>
    </section>
  </main>
</template>

<style scoped>
.login-page { display: grid; grid-template-columns: minmax(0,1.3fr) minmax(360px,.7fr); min-height: 100vh; color: #eaf4f2; background: #0d1729; }
.login-story { position: relative; display: flex; padding: 11vh 8vw; flex-direction: column; justify-content: center; overflow: hidden; background: radial-gradient(circle at 70% 35%,rgba(43,165,143,.2),transparent 28%),linear-gradient(145deg,#101c31,#0a1322); }
.login-brand { color: #6bd0bc; font-size: 12px; font-weight: 700; letter-spacing: .2em; }.login-story h1 { position: relative; z-index: 2; margin: 28px 0 18px; font-size: clamp(46px,6vw,86px); line-height: 1.08; letter-spacing: -.055em; }.login-story h1 em { color: #48b9a5; font-style: normal; }.login-story p { position: relative; z-index: 2; max-width: 620px; color: #9dafba; font-size: 17px; line-height: 1.9; }
.login-orbit { position: absolute; right: -110px; bottom: -160px; width: 520px; height: 520px; border: 1px solid rgba(99,218,194,.18); border-radius: 50%; }.login-orbit i { position: absolute; inset: 70px; border: 1px solid rgba(99,218,194,.14); border-radius: 50%; }.login-orbit i:nth-child(2) { inset: 145px; }.login-orbit i:nth-child(3) { inset: 215px; }.login-orbit strong { position: absolute; top: 225px; left: 227px; color: #55c8b3; font-size: 52px; }
.login-card { display: flex; padding: 8vh 6vw; flex-direction: column; align-items: center; justify-content: center; color: #1b293d; background: #f5f8f8; text-align: center; }.login-card header { display: flex; width: min(360px,100%); align-items: center; justify-content: space-between; margin-bottom: 28px; }.login-card header span { font-size: 20px; font-weight: 750; }.login-card header small { color: #8b98a4; font-size: 10px; letter-spacing: .16em; }.qr-shell { display: grid; width: 300px; height: 300px; padding: 14px; place-items: center; border: 1px solid #dce6e4; border-radius: 30px; background: #fff; box-shadow: 0 22px 60px rgba(20,44,50,.12); transition: filter .2s,opacity .2s; }.qr-shell.muted { opacity: .35; filter: blur(2px); }.qr-shell img { width: 100%; height: 100%; border-radius: 18px; }.qr-loading { color: #87949f; font-size: 13px; }.login-card h2 { margin: 28px 0 8px; font-size: 25px; }.login-card > p { max-width: 350px; margin: 0; color: #7a8793; font-size: 13px; line-height: 1.7; }.login-error { color: #d95061 !important; }.login-card button { margin-top: 20px; padding: 11px 24px; border: 0; border-radius: 13px; color: #fff; background: #2ba58f; cursor: pointer; }.login-card footer { margin-top: 34px; color: #9aa4ad; font-size: 11px; }.online-dot { display: inline-block; width: 7px; height: 7px; margin-right: 5px; border-radius: 50%; background: #2ba58f; }
@media (max-width: 800px) { .login-page { grid-template-columns: 1fr; }.login-story { display: none; }.login-card { min-height: 100vh; padding: 40px 22px; } }
</style>
