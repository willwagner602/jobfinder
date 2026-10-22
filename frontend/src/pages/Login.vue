<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { apiSend, errorMessage, refreshSession } from '../api';
const router = useRouter();
const form = reactive({ email: '', password: '', code: '' });
const codeMode = ref(false);
const loading = ref(false), sending = ref(false), error = ref(''), notice = ref('');
async function login() {
  error.value = ''; notice.value = '';
  if (!form.email.trim()) { error.value = 'Email is required.'; return; }
  if (!form.password && !(codeMode.value && form.code)) { error.value = codeMode.value ? 'Enter your one-time code.' : 'Enter your password.'; return; }
  loading.value = true;
  try {
    await apiSend('/api/login', 'POST', { email: form.email, password: codeMode.value ? undefined : form.password, code: codeMode.value ? form.code : undefined });
    await refreshSession();
    router.push('/dashboard');
  } catch (err) { error.value = errorMessage(err); }
  finally { loading.value = false; }
}
async function sendCode() {
  error.value = ''; notice.value = '';
  if (!form.email.trim()) { error.value = 'Email is required before requesting a code.'; return; }
  sending.value = true;
  try {
    await apiSend('/api/login', 'POST', { email: form.email, sendCode: true });
    codeMode.value = true;
    notice.value = 'A login code was sent if the account is ready.';
  } catch (err) { error.value = errorMessage(err); }
  finally { sending.value = false; }
}
</script>
<template>
  <section class="auth-layout">
    <header class="auth-intro">
      <p class="eyebrow">Welcome back</p>
      <h1>Log in</h1>
      <p class="muted">Sign in to review your dashboard and job matches.</p>
    </header>
    <form class="card form auth-form" @submit.prevent="login" aria-labelledby="login-title" novalidate>
      <h2 id="login-title" class="text-2xl">{{ codeMode ? 'Log in with a code' : 'Log in with your password' }}</h2>
      <p v-if="error" class="alert error" role="alert">{{ error }}</p>
      <p v-if="notice" class="alert success" role="status">{{ notice }}</p>
      <div class="field"><label for="login-email">Email</label><input id="login-email" v-model="form.email" class="input" type="email" autocomplete="email" required /></div>
      <div v-if="!codeMode" class="field"><label for="login-password">Password</label><input id="login-password" v-model="form.password" class="input" type="password" autocomplete="current-password" required /></div>
      <div v-else class="field"><label for="login-code">One-time code</label><input id="login-code" v-model="form.code" class="input" inputmode="numeric" autocomplete="one-time-code" required /></div>
      <div class="auth-actions">
        <button class="button auth-submit" :disabled="loading">{{ loading ? 'Logging in…' : 'Log in' }}</button>
        <button v-if="!codeMode" class="button code-option" type="button" :disabled="sending" @click="sendCode">{{ sending ? 'Sending…' : 'Send me a one time code' }}</button>
        <button v-else class="button code-option" type="button" @click="codeMode = false; error = ''; notice = ''">Use password instead</button>
      </div>
    </form>
  </section>
</template>
