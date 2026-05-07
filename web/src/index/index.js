import Alpine from 'alpinejs';
import { getJSON, postJSON } from '../shared/api.js';

export function createSessionsPage() {
  return {
    query: '',
    modal: false,
    path: '',
    recent: [],
    creating: false,
    error: '',

    subscribe() {
      try {
        const es = new EventSource('/events?id=__all__');
        es.onmessage = (e) => {
          if (e.data === 'new-session') window.location.reload();
        };
      } catch {}
    },

    filter() {
      const q = this.query.toLowerCase();
      document.querySelectorAll('.session-card').forEach((card) => {
        const match = card.dataset.search.toLowerCase().includes(q);
        card.classList.toggle('hidden', !match);
      });
      document.querySelectorAll('.project-group').forEach((group) => {
        const anyVisible = group.querySelector('.session-card:not(.hidden)') !== null;
        group.style.display = anyVisible ? '' : 'none';
      });
    },

    async openModal() {
      this.modal = true;
      this.path = '';
      this.error = '';
      this.recent = [];
      this.$nextTick(() => this.$refs.sessionPath.focus());
      try {
        const response = await getJSON('/api/recent-locations');
        this.recent = (response.locations || []).slice(0, 10);
      } catch {}
    },

    async create() {
      const p = this.path.trim();
      if (!p) {
        this.error = 'Please enter a path';
        return;
      }
      this.creating = true;
      this.error = '';
      try {
        const response = await postJSON('/api/new-session', { path: p });
        if (response.ok && response.id) {
          window.location = '/session?id=' + encodeURIComponent(response.id);
          return;
        }
        this.error = response.error || 'Failed to create session';
      } catch (error) {
        this.error = error.message || 'Network error';
      } finally {
        this.creating = false;
      }
    }
  };
}

window.sessionsPage = createSessionsPage;
window.Alpine = Alpine;
Alpine.start();
