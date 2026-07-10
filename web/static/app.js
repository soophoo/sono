(function () {
    function openModal(question, onConfirm) {
        var overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.innerHTML =
            '<div class="modal" role="dialog" aria-modal="true">' +
            '<p class="modal-message"></p>' +
            '<div class="modal-actions">' +
            '<button type="button" class="modal-cancel">Cancel</button>' +
            '<button type="button" class="modal-confirm">Confirm</button>' +
            '</div></div>';
        overlay.querySelector('.modal-message').textContent = question;

        function close() {
            overlay.remove();
            document.removeEventListener('keydown', onKey);
        }
        function onKey(e) {
            if (e.key === 'Escape') close();
        }
        overlay.querySelector('.modal-cancel').addEventListener('click', close);
        overlay.querySelector('.modal-confirm').addEventListener('click', function () {
            close();
            onConfirm();
        });
        overlay.addEventListener('click', function (e) {
            if (e.target === overlay) close();
        });
        document.addEventListener('keydown', onKey);
        document.body.appendChild(overlay);
        overlay.querySelector('.modal-confirm').focus();
    }

    document.addEventListener('htmx:confirm', function (e) {
        if (!e.detail.question) return;
        e.preventDefault();
        openModal(e.detail.question, function () {
            e.detail.issueRequest(true);
        });
    });

    function showToast(message, level) {
        var container = document.getElementById('toast-container');
        if (!container) {
            container = document.createElement('div');
            container.id = 'toast-container';
            document.body.appendChild(container);
        }
        var toast = document.createElement('div');
        toast.className = 'toast toast-' + (level === 'error' ? 'error' : 'success');
        toast.textContent = message;
        container.appendChild(toast);
        requestAnimationFrame(function () {
            toast.classList.add('toast-show');
        });
        setTimeout(function () {
            toast.classList.remove('toast-show');
            setTimeout(function () {
                toast.remove();
            }, 300);
        }, 3500);
    }

    document.body.addEventListener('toast', function (e) {
        if (e.detail && e.detail.message) showToast(e.detail.message, e.detail.level);
    });
})();
