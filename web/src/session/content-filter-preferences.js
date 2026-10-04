import { loadJSON, saveJSON } from '../shared/storage.js';
import { normalizeContentFilter } from './content-filter.js';

const KEY = 'pi-web:session-content-filter';

export function readContentFilter() {
  return normalizeContentFilter(loadJSON(KEY, null));
}

export function saveContentFilter(filter) {
  saveJSON(KEY, normalizeContentFilter(filter));
}
