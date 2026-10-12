<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { RouterLink, RouterView, useRouter } from 'vue-router';
import { BriefcaseBusiness, LogOut, Moon, Sun } from 'lucide-vue-next';
import { apiSend, errorMessage, refreshSession, session } from './api';
import { followSystemTheme, hasManualTheme, initializeTheme, setTheme, theme } from './theme';

const router = useRouter();
const themeLabel = computed(() => (hasManualTheme.value ? `${theme.value} theme selected` : `following system (${theme.value})`));

onMounted(async () => {
  initializeTheme();
  try {
    await refreshSession();
  } catch {
    session.loading = false;
  }
});

async function logout() {
  try {
    await apiSend('/api/logout', 'POST', {});
    await refreshSession();
    router.push('/login');
  } catch (err) {
    alert(errorMessage(err));
  }
}
</script>

<template>
  <div class="min-h-screen shell-bg text-foreground">
    <header class="sticky top-0 z-20 border-b bg-card/95 backdrop-blur">
      <div class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-4">
        <RouterLink to="/" class="brand" aria-label="jobs.willwagner.me dashboard">
          <BriefcaseBusiness class="h-6 w-6" aria-hidden="true" />
          <span>jobs.willwagner.me</span>
        </RouterLink>
        <nav class="flex flex-wrap items-center gap-2" aria-label="Primary navigation">
          <RouterLink v-if="session.authenticated" to="/" class="nav-link">Dashboard</RouterLink>
          <RouterLink v-if="session.authenticated" to="/onboarding" class="nav-link">Profile</RouterLink>
          <RouterLink v-if="session.authenticated" to="/jobs" class="nav-link">Job matches</RouterLink>
          <RouterLink v-if="!session.authenticated" to="/signup" class="nav-link">Sign up</RouterLink>
          <RouterLink v-if="!session.authenticated" to="/login" class="nav-link">Log in</RouterLink>
          <button class="icon-button" :aria-label="themeLabel" title="Toggle light or dark theme" @click="setTheme(theme === 'dark' ? 'light' : 'dark')">
            <Sun v-if="theme === 'dark'" class="h-4 w-4" aria-hidden="true" />
            <Moon v-else class="h-4 w-4" aria-hidden="true" />
          </button>
          <button v-if="hasManualTheme" class="button ghost" type="button" @click="followSystemTheme">Use system theme</button>
          <button v-if="session.authenticated" class="button secondary" type="button" @click="logout">
            <LogOut class="h-4 w-4" aria-hidden="true" /> Log out
          </button>
        </nav>
      </div>
    </header>
    <main class="mx-auto max-w-6xl px-4 py-8">
      <RouterView />
    </main>
  </div>
</template>
