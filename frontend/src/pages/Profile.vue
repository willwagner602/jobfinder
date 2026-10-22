<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { apiGet, apiSend, errorMessage, Profile } from '../api';
const form = reactive({ resumeText: '', experienceText: '', desiredTitles: '', searchPreferences: '', blockedCompanies: '', matcherName: '' });
const loading = ref(true), saving = ref(false), error = ref(''), success = ref('');
function fill(profile: Profile) { Object.assign(form, { resumeText: profile.resumeText || '', experienceText: profile.experienceText || '', desiredTitles: profile.desiredTitles || '', searchPreferences: profile.searchPreferences || '', blockedCompanies: profile.blockedCompanies || '', matcherName: profile.matcherName || '' }); }
onMounted(async () => { try { const data = await apiGet<{ profile: Profile }>('/api/profile'); fill(data.profile || {}); } catch (err) { error.value = errorMessage(err); } finally { loading.value = false; } });
async function save() {
  error.value = ''; success.value = '';
  if (!form.resumeText.trim() && !form.experienceText.trim()) { error.value = 'Resume text or experience bullets are required.'; return; }
  saving.value = true;
  try { const data = await apiSend<{ profile: Profile; message?: string }>('/api/profile', 'PUT', form); fill(data.profile || form); success.value = data.message || 'Profile saved.'; }
  catch (err) { error.value = errorMessage(err); }
  finally { saving.value = false; }
}
</script>
<template>
  <section class="page-stack page-narrow">
    <header class="page-intro"><p class="eyebrow">Profile and onboarding</p><h1>Profile and matching inputs</h1><p class="muted">These fields are saved by the server and used to request a prior-week match run.</p></header>
    <p v-if="loading" class="card" role="status">Loading profile…</p>
    <form v-else class="card form profile-form" @submit.prevent="save" aria-labelledby="profile-title">
      <h2 id="profile-title" class="text-2xl">Edit profile</h2>
      <p v-if="error" class="alert error" role="alert">{{ error }}</p><p v-if="success" class="alert success" role="status">{{ success }}</p>
      <div class="field"><label for="resume-text">Resume text</label><textarea id="resume-text" v-model="form.resumeText" class="textarea" rows="8" /></div>
      <div class="field"><label for="experience-text">Experience bullets</label><textarea id="experience-text" v-model="form.experienceText" class="textarea" rows="6" /></div>
      <div class="profile-secondary-fields">
        <div class="field"><label for="desired-titles">Desired titles, one per line</label><textarea id="desired-titles" v-model="form.desiredTitles" class="textarea" rows="4" /></div>
        <div class="field"><label for="search-preferences">Search preferences JSON or notes</label><textarea id="search-preferences" v-model="form.searchPreferences" class="textarea" rows="4" /></div>
        <div class="field"><label for="blocked-companies">Blocked companies, one per line</label><textarea id="blocked-companies" v-model="form.blockedCompanies" class="textarea" rows="4" /></div>
        <div class="field"><label for="matcher-name">Matcher override</label><input id="matcher-name" v-model="form.matcherName" class="input" /></div>
      </div>
      <div class="form-actions"><button class="button" :disabled="saving">{{ saving ? 'Saving…' : 'Save and request prior-week match run' }}</button></div>
    </form>
  </section>
</template>
