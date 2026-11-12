/* ============================================================
 * 百度网盘清理规则（JS + CSS）
 *
 * 把本文件与 BaiduNetdiskRunner.exe 放在同一目录即可。
 * 修改后重启客户端生效，不需要重新编译 exe。
 *
 * 在页面里执行（不是 Node 环境，不要用 require）。
 * 样式规则已合并到下面的 CSS 常量中。
 * ============================================================ */
(function () {
  if (window.__ndAdblock) return;
  window.__ndAdblock = true;

  var CSS = `
/* 头部右侧只保留最后一项 More 菜单，其他项全部隐藏；以后增删项无需改规则 */
.wp-disk-header.personal-pc .wp-disk-header__right.clearfix > .wp-disk-header__right-item:not(:last-child) {
  display: none !important;
}

/* 文件列表里的推荐卡片 */
.all-file-recommend-area,
.recommend-card-transition-wrap { display: none !important; }

/* SVIP / 活动 / 会员卡弹窗 */
.doc-save-svip-dialog,
.wp-dialog-data-payguide,
.ee-activity-wrap,
.uip-membership-card,
.uip-upgrade-btn-wrap,
.u-svip-button { display: none !important; }

/* 保留几类旧规则的兜底（若新版本改了类名） */
[class*="ad-guide"],
[class*="ad-factory"],
[class*="activity-conf"],
[class*="ai-tool-banner"],
[class*="svip-quota-banner"],
[class*="privilege-guide"],
[class*="video-payment-toast"] { display: none !important; }
`;

  var SELECTORS = [
    '.all-file-recommend-area',
    '.recommend-card-transition-wrap',
    '.doc-save-svip-dialog',
    '.wp-dialog-data-payguide',
    '.ee-activity-wrap',
    '.uip-membership-card',
    '.uip-upgrade-btn-wrap',
    '.u-svip-button'
  ];

  // 搜索框默认文案常被换成热播剧推广，统一改成中性文案
  var PLACEHOLDER_TEXT = '搜索我的网盘';

  function injectCSS() {
    try {
      var style = document.getElementById('nd-adblock-style');
      if (!style) {
        style = document.createElement('style');
        style.id = 'nd-adblock-style';
      }
      if (style.textContent !== CSS) style.textContent = CSS;
      var parent = document.head || document.documentElement;
      if (parent && style.parentNode !== parent) parent.appendChild(style);
    } catch (e) { }
  }

  function hide(el) {
    try { el.style.setProperty('display', 'none', 'important'); } catch (e) { }
  }

  function fixPlaceholder() {
    var inputs;
    try { inputs = document.querySelectorAll('.global-search-wrap input, .tags-input-ipt input'); } catch (e) { return; }
    for (var i = 0; i < inputs.length; i++) {
      try {
        if (inputs[i].getAttribute('placeholder') !== PLACEHOLDER_TEXT) {
          inputs[i].setAttribute('placeholder', PLACEHOLDER_TEXT);
        }
      } catch (e) { }
    }
  }

  function clean() {
    var i, els;
    var header;
    try {
      header = document.querySelector('.wp-disk-header.personal-pc .wp-disk-header__right.clearfix');
    } catch (e) { header = null; }
    if (header) {
      for (i = 0; i < header.children.length - 1; i++) hide(header.children[i]);
    }
    for (i = 0; i < SELECTORS.length; i++) {
      try { els = document.querySelectorAll(SELECTORS[i]); } catch (e) { continue; }
      for (var j = 0; j < els.length; j++) hide(els[j]);
    }
  }

  function start() {
    injectCSS();
    clean();
    fixPlaceholder();
    try { setInterval(fixPlaceholder, 2000); } catch (e) { }
    try {
      var timer = null;
      var schedule = function () {
        if (timer) return;
        timer = setTimeout(function () { timer = null; injectCSS(); clean(); }, 150);
      };
      var mo = new MutationObserver(schedule);
      mo.observe(document.documentElement || document, { childList: true, subtree: true });
    } catch (e) { /* ignore */ }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();
