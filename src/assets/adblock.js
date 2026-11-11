/* ============================================================
 * 百度网盘清理规则（JS）
 *
 * 在页面里执行（不是 Node 环境，不要用 require）。
 * 处理动态插入/重渲染的元素；静态规则主要在 adblock.css。
 *
 * 改完后运行 start.bat 同步到客户端。
 * ============================================================ */
(function () {
  if (window.__ndAdblock) return;
  window.__ndAdblock = true;

  var SELECTORS = [
    '[id^="pc-header-ad-"]',
    '.wp-header-game',
    '.wp-disk-header__vip-btn',
    '.vip-cycle-guide',
    '.vip-cycle-guide__popper',
    '.header-capacity-info .price-text',
    '.header-expand-capacity-btn',
    '.wp-header-user__switch-cert-tip',
    '.smart-recommend-wrap',
    '.all-file-recommend-area',
    '.recommend-card-transition-wrap',
    '.doc-save-svip-dialog',
    '.wp-dialog-data-payguide',
    '.ee-activity-wrap',
    '.uip-membership-card',
    '.uip-upgrade-btn-wrap',
    '.u-svip-button',
    '.lead-tip-box',
    '.mac-header-lead-tip',
    '.uip-trigger-vip-badge-wrap',
    '.uip-operations',
    '.uip-operation-item',
    '[class*="reward-task"]',
    '[class*="reward-popover"]'
  ];

  // 服务端广告有时不带 id，用文案兜底
  var TEXT_RE = /^(瓜分百万.*SVIP|空间扩容.*|.*G起)$/;
  var TEXT_SCOPE = '.wp-disk-header__right, .vip-btn-content, .header-capacity-section';

  // 头像下拉面板等动态广告：按文案找叶子节点，再往上找可隐藏的容器
  var AD_TEXT_RE = /^(瓜分百万.*SVIP|开通SVIP|成为超级会员|立即续费|年卡惊喜价.*)$/;
  var AD_CONTAINER_RE = /(^|\s)(uip-|ad-|advert|banner|card|item|popover|activity|promo|gift|reward)/i;

  // 搜索框默认文案常被换成热播剧推广，统一改成中性文案
  var PLACEHOLDER_TEXT = '搜索我的网盘';

  function hide(el) {
    try { el.style.setProperty('display', 'none', 'important'); } catch (e) { }
  }

  function hideAdParents() {
    var ads;
    try { ads = document.querySelectorAll('[id^="pc-header-ad-"]'); } catch (e) { return; }
    for (var i = 0; i < ads.length; i++) {
      hide(ads[i]);
      try {
        var p = ads[i].closest('.wp-disk-header__right-item');
        if (p) hide(p);
      } catch (e) { }
    }
  }

  function hideByText() {
    var roots;
    try {
      roots = document.querySelectorAll('.user-info-popover-adapter, .wp-disk-header, .u-tooltip-popper, .u-popover');
    } catch (e) { return; }
    for (var r = 0; r < roots.length; r++) {
      var els;
      try { els = roots[r].querySelectorAll('a, button, span, div'); } catch (e) { continue; }
      for (var i = 0; i < els.length; i++) {
        var el = els[i];
        if (el.children.length > 0) continue;
        if (el.style && el.style.display === 'none') continue;
        var t = '';
        try { t = (el.textContent || '').replace(/\s+/g, ' ').trim(); } catch (e) { continue; }
        if (!t || t.length > 40 || !AD_TEXT_RE.test(t)) continue;
        var p = el;
        for (var k = 0; k < 4 && p; k++) {
          var cls = '';
          try { cls = (p.className && p.className.toString()) || ''; } catch (e) { }
          if ((p.matches && p.matches('a, button')) || AD_CONTAINER_RE.test(cls)) {
            hide(p);
            break;
          }
          p = p.parentElement;
        }
        hide(el);
      }
    }
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
    for (i = 0; i < SELECTORS.length; i++) {
      try { els = document.querySelectorAll(SELECTORS[i]); } catch (e) { continue; }
      for (var j = 0; j < els.length; j++) hide(els[j]);
    }
    hideAdParents();
    hideByText();
    try {
      els = document.querySelectorAll(TEXT_SCOPE);
      for (i = 0; i < els.length; i++) {
        var kids = els[i].querySelectorAll('a, span, div');
        for (var k = 0; k < kids.length; k++) {
          var t = (kids[k].textContent || '').replace(/\s+/g, ' ').trim();
          if (t && t.length < 40 && TEXT_RE.test(t)) hide(kids[k]);
        }
      }
    } catch (e) { }
  }

  function start() {
    clean();
    fixPlaceholder();
    try { setInterval(fixPlaceholder, 2000); } catch (e) { }
    try {
      var timer = null;
      var schedule = function () {
        if (timer) return;
        timer = setTimeout(function () { timer = null; clean(); }, 150);
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