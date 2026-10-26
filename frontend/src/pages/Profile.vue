<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { apiGet, apiSend, csrfToken, DocumentInfo, errorMessage, Profile } from '../api';
const form = reactive({ desiredTitles: [] as string[], blockedCompanies: [] as string[] });
const documents = ref<DocumentInfo[]>([]);
const titleQuery = ref(''), companyQuery = ref('');
const titles = ref<string[]>([]), companies = ref<string[]>([]);
const loading = ref(true), uploading = ref(false), saving = ref(false), requestingRun = ref(false), firstSave = ref(true), error = ref(''), success = ref('');
let suggestionTimer: ReturnType<typeof setTimeout> | undefined;
async function loadSuggestions(kind: 'titles' | 'companies', query: string) {
  if (!query.trim()) { (kind === 'titles' ? titles : companies).value = []; return; }
  try {
    const data = await apiGet<{ items: string[] }>(`/api/${kind}?q=${encodeURIComponent(query)}`);
    (kind === 'titles' ? titles : companies).value = data.items;
  } catch (err) { error.value = errorMessage(err); }
}
function search(kind: 'titles' | 'companies', query: string) {
  if (suggestionTimer) clearTimeout(suggestionTimer);
  suggestionTimer = setTimeout(() => void loadSuggestions(kind, query), 150);
}
function select(kind: 'titles' | 'companies', value: string) {
  const selected = kind === 'titles' ? form.desiredTitles : form.blockedCompanies;
  if (!selected.includes(value)) selected.push(value);
  if (kind === 'titles') { titleQuery.value = ''; titles.value = []; }
  else { companyQuery.value = ''; companies.value = []; }
}
function remove(kind: 'titles' | 'companies', value: string) {
  const selected = kind === 'titles' ? form.desiredTitles : form.blockedCompanies;
  const index = selected.indexOf(value);
  if (index >= 0) selected.splice(index, 1);
}
async function loadDocuments() {
  const data = await apiGet<{ documents: DocumentInfo[] }>('/api/documents');
  documents.value = data.documents;
}
onMounted(async () => {
  try {
    const [data] = await Promise.all([apiGet<{ profile: Profile }>('/api/profile'), loadDocuments()]);
    form.desiredTitles = parseList(data.profile?.desiredTitles);
    form.blockedCompanies = parseList(data.profile?.blockedCompanies);
    firstSave.value = !data.profile?.hasProfile;
  } catch (err) { error.value = errorMessage(err); }
  finally { loading.value = false; }
});
function parseList(value?: string) {
  try { const parsed = JSON.parse(value || '[]'); return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : []; }
  catch { return value ? value.split('\n').filter(Boolean) : []; }
}
async function uploadFiles(event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files || []);
  if (!files.length) return;
  error.value = ''; success.value = ''; uploading.value = true;
  try {
    for (const file of files) {
      const formData = new FormData();
      formData.append('file', file);
      const response = await fetch('/api/documents', {
        method: 'POST', credentials: 'include',
        headers: { 'X-CSRF-Token': await csrfToken() }, body: formData,
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(result?.error?.message || 'Document upload failed.');
    }
    await loadDocuments();
    success.value = `${files.length} document${files.length === 1 ? '' : 's'} uploaded.`;
  } catch (err) { error.value = errorMessage(err); await loadDocuments().catch(() => undefined); }
  finally { uploading.value = false; input.value = ''; }
}
async function save() {
  error.value = ''; success.value = ''; saving.value = true;
  try {
    const data = await apiSend<{ profile: Profile; message?: string }>('/api/profile', 'PUT', {
      desiredTitles: form.desiredTitles.join('\n'), blockedCompanies: form.blockedCompanies.join('\n'),
    });
    firstSave.value = false;
    success.value = data.message || 'Profile saved.';
  } catch (err) { error.value = errorMessage(err); }
  finally { saving.value = false; }
}
async function requestMatchRun() {
  error.value = ''; success.value = ''; requestingRun.value = true;
  try {
    const result = await apiSend<{ message: string }>('/api/match-runs', 'POST', {});
    success.value = result.message;
  } catch (err) { error.value = errorMessage(err); }
  finally { requestingRun.value = false; }
}
</script>
<template>
  <section class="page-stack page-narrow">
    <header class="page-intro"><p class="eyebrow">Profile and onboarding</p><h1>Profile and matching inputs</h1><p class="muted">Upload documents and choose the roles and companies relevant to your search.</p></header>
    <p v-if="loading" class="card" role="status">Loading profile…</p>
    <form v-else class="card form profile-form" @submit.prevent="save" aria-labelledby="profile-title">
      <h2 id="profile-title" class="text-2xl">Profile and documents</h2>
      <p v-if="error" class="alert error" role="alert">{{ error }}</p><p v-if="success" class="alert success" role="status">{{ success }}</p>
      <div class="field"><span class="field-label">Documents</span><div class="document-upload-row">
        <label class="button" for="document-upload">{{ uploading ? 'Uploading…' : 'Upload Documents' }}</label>
        <input id="document-upload" class="visually-hidden" type="file" multiple :disabled="uploading" accept=".pdf,.doc,.docx,.odt,.ods,.odp,.rtf,.txt,.ppt,.pptx,.xls,.xlsx,.md,.csv,application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.oasis.opendocument.text,text/plain" @change="uploadFiles" />
        <ul v-if="documents.length" class="document-list"><li v-for="document in documents" :key="document.id">{{ document.filename }} <span class="muted">({{ Math.ceil(document.sizeBytes / 1024) }} KB)</span></li></ul>
        <p v-else class="muted document-empty">No documents uploaded yet.</p>
      </div><small class="muted">PDF, Word, OpenDocument, text, and related office formats. Maximum 25 MB per file.</small></div>
      <div class="field"><label for="title-search">Desired titles</label><div class="autocomplete"><input id="title-search" v-model="titleQuery" class="input" autocomplete="off" placeholder="Search job titles…" @input="search('titles', titleQuery)" @keydown.enter.prevent="titles[0] && select('titles', titles[0])" /><ul v-if="titles.length" class="suggestion-list" role="listbox"><li v-for="title in titles" :key="title"><button type="button" role="option" @click="select('titles', title)">{{ title }}</button></li></ul></div><ul v-if="form.desiredTitles.length" class="selected-list"><li v-for="title in form.desiredTitles" :key="title">{{ title }}<button type="button" :aria-label="`Remove ${title}`" @click="remove('titles', title)">×</button></li></ul><small class="muted">Start typing for suggestions.</small></div>
      <div class="field"><label for="company-search">Blocked companies</label><div class="autocomplete"><input id="company-search" v-model="companyQuery" class="input" autocomplete="off" placeholder="Search companies…" @input="search('companies', companyQuery)" @keydown.enter.prevent="companies[0] && select('companies', companies[0])" /><ul v-if="companies.length" class="suggestion-list" role="listbox"><li v-for="company in companies" :key="company"><button type="button" role="option" @click="select('companies', company)">{{ company }}</button></li></ul></div><ul v-if="form.blockedCompanies.length" class="selected-list"><li v-for="company in form.blockedCompanies" :key="company">{{ company }}<button type="button" :aria-label="`Remove ${company}`" @click="remove('companies', company)">×</button></li></ul><small class="muted">Start typing for suggestions.</small></div>
      <div class="form-actions profile-actions"><button class="button" :disabled="saving">{{ saving ? 'Saving…' : 'Save profile' }}</button><button v-if="!firstSave" class="button secondary" type="button" :disabled="requestingRun || saving" @click="requestMatchRun">{{ requestingRun ? 'Requesting…' : 'Run prior-week job match' }}</button></div>
    </form>
  </section>
</template>
