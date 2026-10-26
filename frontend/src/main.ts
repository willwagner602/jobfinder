import { createApp } from 'vue';
import { createRouter, createWebHistory } from 'vue-router';
import App from './App.vue';
import Landing from './pages/Landing.vue';
import Dashboard from './pages/Dashboard.vue';
import Signup from './pages/Signup.vue';
import Verify from './pages/Verify.vue';
import Login from './pages/Login.vue';
import Profile from './pages/Profile.vue';
import JobsList from './pages/JobsList.vue';
import JobDetail from './pages/JobDetail.vue';
import { refreshSession, session } from './api';
import './styles.css';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Landing },
    { path: '/dashboard', component: Dashboard, meta: { requiresAuth: true } },
    { path: '/signup', component: Signup },
    { path: '/verify', component: Verify },
    { path: '/login', component: Login },
    { path: '/onboarding', component: Profile, meta: { requiresAuth: true } },
    { path: '/profile', redirect: '/onboarding' },
    { path: '/jobs', component: JobsList, meta: { requiresAuth: true } },
    { path: '/jobs/:matchId', component: JobDetail, meta: { requiresAuth: true } },
  ],
});

router.beforeEach(async (to) => {
  if (['/', '/signup', '/verify', '/login'].includes(to.path)) return true;
  try {
    if (session.loading || !session.csrfToken) await refreshSession();
  } catch {
    session.loading = false;
    session.authenticated = false;
  }
  if (!session.authenticated) return { path: '/login', query: { redirect: to.fullPath } };
  return true;
});

createApp(App).use(router).mount('#app');
