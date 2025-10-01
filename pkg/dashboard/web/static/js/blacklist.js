// Blacklist Management JavaScript
(function() {
    'use strict';

    let currentEntries = [];
    let editingEntryId = null;

    // DOM elements
    const addEntryBtn = document.getElementById('add-entry-btn');
    const refreshBtn = document.getElementById('refresh-btn');
    const typeFilter = document.getElementById('type-filter');
    const actionFilter = document.getElementById('action-filter');
    const searchInput = document.getElementById('search-input');
    const filterBtn = document.getElementById('filter-btn');
    const clearFiltersBtn = document.getElementById('clear-filters-btn');
    const blacklistTable = document.getElementById('blacklist-table');
    const entryModal = document.getElementById('entry-modal');
    const entryForm = document.getElementById('entry-form');
    const modalTitle = document.getElementById('modal-title');
    const modalClose = document.getElementById('modal-close');
    const modalCancel = document.getElementById('modal-cancel');
    const modalSave = document.getElementById('modal-save');

    // Initialize the page
    function init() {
        bindEvents();
        loadEntries();
        loadStats();
    }

    // Bind event handlers
    function bindEvents() {
        if (addEntryBtn) {
            addEntryBtn.addEventListener('click', openAddModal);
        }

        if (refreshBtn) {
            refreshBtn.addEventListener('click', refreshPage);
        }

        if (filterBtn) {
            filterBtn.addEventListener('click', applyFilters);
        }

        if (clearFiltersBtn) {
            clearFiltersBtn.addEventListener('click', clearFilters);
        }

        if (modalClose) {
            modalClose.addEventListener('click', closeModal);
        }

        if (modalCancel) {
            modalCancel.addEventListener('click', closeModal);
        }

        if (entryForm) {
            entryForm.addEventListener('submit', saveEntry);
        }

        // Click outside modal to close
        if (entryModal) {
            entryModal.addEventListener('click', function(e) {
                if (e.target === entryModal) {
                    closeModal();
                }
            });
        }

        // Enter key in search input
        if (searchInput) {
            searchInput.addEventListener('keypress', function(e) {
                if (e.key === 'Enter') {
                    applyFilters();
                }
            });
        }

        // Auto-refresh every 30 seconds
        setInterval(loadStats, 30000);
    }

    // Load blacklist entries
    function loadEntries(filters = {}) {
        const params = new URLSearchParams();

        if (filters.type) params.append('type', filters.type);
        if (filters.action) params.append('action', filters.action);
        if (filters.search) params.append('search', filters.search);

        const url = `/api/blacklist${params.toString() ? '?' + params.toString() : ''}`;

        fetch(url)
            .then(response => response.json())
            .then(data => {
                if (data.data) {
                    currentEntries = data.data;
                    updateTable(currentEntries);
                    updateEntryCount(data.count || currentEntries.length);
                }
            })
            .catch(error => {
                console.error('Failed to load blacklist entries:', error);
                showNotification('Failed to load blacklist entries', 'error');
            });
    }

    // Load statistics
    function loadStats() {
        fetch('/api/blacklist/stats')
            .then(response => response.json())
            .then(data => {
                if (data.data) {
                    updateStats(data.data);
                }
            })
            .catch(error => {
                console.error('Failed to load blacklist stats:', error);
            });
    }

    // Update table with entries
    function updateTable(entries) {
        const tbody = blacklistTable.querySelector('tbody');
        if (!tbody) return;

        if (entries.length === 0) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="7" style="text-align: center; color: #666; padding: 2rem;">
                        No blacklist entries found
                    </td>
                </tr>
            `;
            return;
        }

        tbody.innerHTML = entries.map(entry => `
            <tr data-entry-id="${entry.id}">
                <td>
                    <span class="badge badge-${entry.type}">${entry.type}</span>
                </td>
                <td>${escapeHtml(entry.value || entry.pattern || '')}</td>
                <td>
                    <span class="action-${entry.action}">${entry.action}</span>
                </td>
                <td title="${escapeHtml(entry.reason || '')}">
                    ${entry.reason && entry.reason.length > 50 ?
                        escapeHtml(entry.reason.substring(0, 50)) + '...' :
                        escapeHtml(entry.reason || '')
                    }
                </td>
                <td>
                    ${entry.enabled ?
                        '<span class="status online">Enabled</span>' :
                        '<span class="status offline">Disabled</span>'
                    }
                </td>
                <td>${formatDate(entry.created_at)}</td>
                <td>
                    <div style="display: flex; gap: 0.5rem;">
                        <button class="btn btn-primary btn-small edit-btn" data-id="${entry.id}">
                            Edit
                        </button>
                        <button class="btn btn-${entry.enabled ? 'warning' : 'success'} btn-small toggle-btn"
                                data-id="${entry.id}" data-action="${entry.enabled ? 'disable' : 'enable'}">
                            ${entry.enabled ? 'Disable' : 'Enable'}
                        </button>
                        <button class="btn btn-danger btn-small delete-btn" data-id="${entry.id}">
                            Delete
                        </button>
                    </div>
                </td>
            </tr>
        `).join('');

        // Bind table action events
        bindTableEvents();
    }

    // Bind table event handlers
    function bindTableEvents() {
        // Edit buttons
        document.querySelectorAll('.edit-btn').forEach(btn => {
            btn.addEventListener('click', function() {
                const entryId = this.getAttribute('data-id');
                editEntry(entryId);
            });
        });

        // Toggle buttons
        document.querySelectorAll('.toggle-btn').forEach(btn => {
            btn.addEventListener('click', function() {
                const entryId = this.getAttribute('data-id');
                const action = this.getAttribute('data-action');
                toggleEntry(entryId, action);
            });
        });

        // Delete buttons
        document.querySelectorAll('.delete-btn').forEach(btn => {
            btn.addEventListener('click', function() {
                const entryId = this.getAttribute('data-id');
                deleteEntry(entryId);
            });
        });
    }

    // Update statistics
    function updateStats(stats) {
        const elements = {
            'blacklist-total': stats.total_entries,
            'blacklist-clientid': stats.entries_by_type?.clientid || 0,
            'blacklist-username': stats.entries_by_type?.username || 0,
            'blacklist-ip': stats.entries_by_type?.ip || 0,
            'blacklist-topic': stats.entries_by_type?.topic || 0
        };

        Object.entries(elements).forEach(([id, value]) => {
            const element = document.getElementById(id);
            if (element) {
                element.textContent = value;
            }
        });
    }

    // Update entry count
    function updateEntryCount(count) {
        const countElements = document.querySelectorAll('.page-header span');
        countElements.forEach(el => {
            if (el.textContent.includes('Total:')) {
                el.textContent = `Total: ${count} entries`;
            }
        });
    }

    // Open add modal
    function openAddModal() {
        editingEntryId = null;
        modalTitle.textContent = 'Add Blacklist Entry';
        entryForm.reset();
        entryModal.style.display = 'block';
    }

    // Open edit modal
    function editEntry(entryId) {
        const entry = currentEntries.find(e => e.id === entryId);
        if (!entry) return;

        editingEntryId = entryId;
        modalTitle.textContent = 'Edit Blacklist Entry';

        // Populate form
        document.getElementById('entry-type').value = entry.type || '';
        document.getElementById('entry-value').value = entry.value || '';
        document.getElementById('entry-pattern').value = entry.pattern || '';
        document.getElementById('entry-action').value = entry.action || '';
        document.getElementById('entry-reason').value = entry.reason || '';
        document.getElementById('entry-description').value = entry.description || '';
        document.getElementById('entry-enabled').checked = entry.enabled !== false;

        if (entry.expires_at) {
            const date = new Date(entry.expires_at);
            document.getElementById('entry-expires').value = date.toISOString().slice(0, 16);
        }

        entryModal.style.display = 'block';
    }

    // Close modal
    function closeModal() {
        entryModal.style.display = 'none';
        editingEntryId = null;
        entryForm.reset();
    }

    // Save entry
    function saveEntry(e) {
        e.preventDefault();

        const formData = new FormData(entryForm);
        const data = {
            type: formData.get('type'),
            value: formData.get('value'),
            pattern: formData.get('pattern'),
            action: formData.get('action'),
            reason: formData.get('reason'),
            description: formData.get('description'),
            enabled: formData.get('enabled') === 'on',
            expires_at: formData.get('expires_at') || null
        };

        // Validation
        if (!data.type || !data.action || !data.reason) {
            showNotification('Please fill in all required fields', 'error');
            return;
        }

        if (!data.value && !data.pattern) {
            showNotification('Please provide either a value or pattern', 'error');
            return;
        }

        const method = editingEntryId ? 'PUT' : 'POST';
        const url = editingEntryId ? `/api/blacklist/${editingEntryId}` : '/api/blacklist';

        fetch(url, {
            method: method,
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        })
        .then(response => response.json())
        .then(result => {
            if (result.code === 0 || response.ok) {
                showNotification(editingEntryId ? 'Entry updated successfully' : 'Entry created successfully', 'success');
                closeModal();
                loadEntries();
                loadStats();
            } else {
                throw new Error(result.message || 'Operation failed');
            }
        })
        .catch(error => {
            console.error('Save error:', error);
            showNotification(error.message || 'Failed to save entry', 'error');
        });
    }

    // Toggle entry enabled/disabled
    function toggleEntry(entryId, action) {
        const data = { enabled: action === 'enable' };

        fetch(`/api/blacklist/${entryId}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        })
        .then(response => response.json())
        .then(result => {
            if (result.code === 0 || response.ok) {
                showNotification(`Entry ${action}d successfully`, 'success');
                loadEntries();
                loadStats();
            } else {
                throw new Error(result.message || 'Operation failed');
            }
        })
        .catch(error => {
            console.error('Toggle error:', error);
            showNotification(error.message || `Failed to ${action} entry`, 'error');
        });
    }

    // Delete entry
    function deleteEntry(entryId) {
        if (!confirm('Are you sure you want to delete this blacklist entry?')) {
            return;
        }

        fetch(`/api/blacklist/${entryId}`, {
            method: 'DELETE'
        })
        .then(response => {
            if (response.ok || response.status === 204) {
                showNotification('Entry deleted successfully', 'success');
                loadEntries();
                loadStats();
            } else {
                throw new Error('Delete failed');
            }
        })
        .catch(error => {
            console.error('Delete error:', error);
            showNotification('Failed to delete entry', 'error');
        });
    }

    // Apply filters
    function applyFilters() {
        const filters = {
            type: typeFilter.value,
            action: actionFilter.value,
            search: searchInput.value.trim()
        };

        loadEntries(filters);
    }

    // Clear filters
    function clearFilters() {
        typeFilter.value = '';
        actionFilter.value = '';
        searchInput.value = '';
        loadEntries();
    }

    // Refresh page
    function refreshPage() {
        loadEntries();
        loadStats();
        showNotification('Data refreshed', 'success');
    }

    // Utility functions
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    function formatDate(dateString) {
        if (!dateString) return '-';
        const date = new Date(dateString);
        return date.toLocaleString();
    }

    function showNotification(message, type = 'info') {
        // Simple notification system
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.textContent = message;
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 1rem;
            border-radius: 4px;
            color: white;
            z-index: 10000;
            max-width: 300px;
            opacity: 0;
            transition: opacity 0.3s;
        `;

        switch (type) {
            case 'success':
                notification.style.backgroundColor = '#28a745';
                break;
            case 'error':
                notification.style.backgroundColor = '#dc3545';
                break;
            default:
                notification.style.backgroundColor = '#007bff';
        }

        document.body.appendChild(notification);

        // Fade in
        setTimeout(() => {
            notification.style.opacity = '1';
        }, 10);

        // Auto remove after 3 seconds
        setTimeout(() => {
            notification.style.opacity = '0';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.parentNode.removeChild(notification);
                }
            }, 300);
        }, 3000);
    }

    // Initialize when DOM is loaded
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();