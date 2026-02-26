// CodeMirror 6 — 增强 textarea 方案（本地打包，无 Alpine 组件依赖）
import { EditorView, basicSetup, EditorState, json, javascript, oneDark, linter, lintGutter } from '/static/js/cm-bundle.js';

(function() {
    try {

        const slateTheme = EditorView.theme({
            '&': { backgroundColor: 'rgb(30 41 59)', borderRadius: '0.5rem', border: '1px solid rgb(51 65 85)', fontSize: '0.875rem' },
            '.cm-gutters': { backgroundColor: 'rgb(30 41 59)', borderRight: '1px solid rgb(51 65 85)', color: 'rgb(100 116 139)' },
            '.cm-activeLineGutter': { backgroundColor: 'rgb(51 65 85)' },
            '&.cm-focused': { outline: '2px solid rgb(99 102 241)', outlineOffset: '-1px' },
            '.cm-cursor': { borderLeftColor: 'rgb(199 210 254)' },
            '.cm-selectionBackground': { backgroundColor: 'rgb(99 102 241 / 0.3) !important' }
        });

        function jsonLinter() {
            return linter(view => {
                const text = view.state.doc.toString();
                if (!text.trim()) return [];
                try { JSON.parse(text); return []; }
                catch (e) {
                    let from = 0, to = text.length;
                    const m = e.message.match(/position (\d+)/i);
                    if (m) { from = +m[1]; to = Math.min(from + 1, text.length); }
                    return [{ from, to, severity: 'error', message: e.message }];
                }
            });
        }

        function createEditor(container, opts = {}) {
            const { doc = '', language = 'json', readonly = false, onChange = null,
                    minHeight = '6rem', maxHeight = '24rem' } = opts;
            const exts = [
                basicSetup, oneDark, slateTheme,
                EditorView.theme({ '.cm-scroller': { minHeight, maxHeight, overflow: 'auto' } })
            ];
            if (language === 'json') { exts.push(json(), lintGutter(), jsonLinter()); }
            else if (language === 'javascript') { exts.push(javascript()); }
            if (readonly) { exts.push(EditorState.readOnly.of(true), EditorView.editable.of(false)); }
            if (onChange && !readonly) {
                exts.push(EditorView.updateListener.of(u => { if (u.docChanged) onChange(u.state.doc.toString()); }));
            }
            return new EditorView({ state: EditorState.create({ doc, extensions: exts }), parent: container });
        }

        // ── 增强 textarea[data-cm] ──
        function enhance(textarea) {
            if (textarea._cmDone) return;
            textarea._cmDone = true;

            const lang = textarea.dataset.cm || 'json';
            const minH = textarea.dataset.cmMin || '6rem';
            const maxH = textarea.dataset.cmMax || '24rem';
            const isJSON = lang === 'json';

            // 创建包装容器
            const wrap = document.createElement('div');
            wrap.className = 'cm-wrap';

            // JSON 格式化按钮
            if (isJSON) {
                const bar = document.createElement('div');
                bar.className = 'flex items-center justify-end gap-2 mb-1';
                const fmtBtn = document.createElement('button');
                fmtBtn.type = 'button';
                fmtBtn.className = 'text-xs text-indigo-400 hover:text-indigo-300';
                fmtBtn.textContent = '格式化';
                const cmpBtn = document.createElement('button');
                cmpBtn.type = 'button';
                cmpBtn.className = 'text-xs text-slate-400 hover:text-slate-300';
                cmpBtn.textContent = '压缩';
                bar.append(fmtBtn, cmpBtn);
                wrap.appendChild(bar);

                fmtBtn.addEventListener('click', () => {
                    try {
                        const p = JSON.parse(editor.state.doc.toString());
                        const v = JSON.stringify(p, null, 2);
                        editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: v } });
                    } catch (_) {}
                });
                cmpBtn.addEventListener('click', () => {
                    try {
                        const p = JSON.parse(editor.state.doc.toString());
                        const v = JSON.stringify(p);
                        editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: v } });
                    } catch (_) {}
                });
            }

            const editorDiv = document.createElement('div');
            wrap.appendChild(editorDiv);

            textarea.parentNode.insertBefore(wrap, textarea);
            textarea.style.display = 'none';

            const editor = createEditor(editorDiv, {
                doc: textarea.value,
                language: lang,
                minHeight: minH,
                maxHeight: maxH,
                onChange(val) {
                    textarea.value = val;
                    textarea.dispatchEvent(new Event('input', { bubbles: true }));
                }
            });

            textarea._cmEditor = editor;
        }

        // ── 增强 [data-cm-viewer] (只读代码查看) ──
        function enhanceViewer(el) {
            if (el._cmDone) return;
            el._cmDone = true;

            const lang = el.dataset.cmViewer || 'json';
            const maxH = el.dataset.cmMax || '24rem';
            const code = el.textContent || '';

            el.textContent = '';
            createEditor(el, { doc: code, language: lang, readonly: true, maxHeight: maxH });
        }

        // ── 扫描并增强 ──
        function scan(root) {
            (root || document).querySelectorAll('textarea[data-cm]:not([data-cm-done])').forEach(enhance);
            (root || document).querySelectorAll('[data-cm-viewer]:not([data-cm-done])').forEach(enhanceViewer);
        }

        // 暴露给 Alpine x-init 调用（处理动态创建的元素）
        window.__cmEnhance = function(el) {
            if (el && el.tagName === 'TEXTAREA' && el.dataset.cm !== undefined) enhance(el);
        };

        // 同步 Alpine model 变更到 CM（当外部代码修改 textarea.value 时调用）
        window.__cmSync = function(el) {
            if (!el || !el._cmEditor) return;
            var current = el._cmEditor.state.doc.toString();
            if (el.value !== current) {
                el._cmEditor.dispatch({ changes: { from: 0, to: current.length, insert: el.value } });
            }
        };

        // 初始扫描
        scan();

        // HTMX 动态加载的内容
        document.addEventListener('htmx:afterSwap', e => setTimeout(() => scan(e.detail.target), 50));

        // MutationObserver 捕获动态创建的 textarea[data-cm]
        new MutationObserver(function(mutations) {
            for (var i = 0; i < mutations.length; i++) {
                var added = mutations[i].addedNodes;
                for (var j = 0; j < added.length; j++) {
                    var node = added[j];
                    if (node.nodeType !== 1) continue;
                    if (node.matches && node.matches('textarea[data-cm]')) enhance(node);
                    if (node.querySelectorAll) {
                        node.querySelectorAll('textarea[data-cm]').forEach(enhance);
                        node.querySelectorAll('[data-cm-viewer]').forEach(enhanceViewer);
                    }
                }
            }
        }).observe(document.body, { childList: true, subtree: true });

    } catch (e) {
        console.error('[CM] CodeMirror 初始化失败:', e);
    }
})();
