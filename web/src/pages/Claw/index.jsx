/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Empty, Spin, Typography } from '@douyinfe/semi-ui';
import { API } from '../../helpers';

const CLAW_LOAD_TIMEOUT_MS = 12000;
const CLAW_API_BASE = '/api/claw';

const persistClawAccessToken = (accessToken) => {
  if (typeof accessToken !== 'string' || accessToken.trim() === '') {
    return;
  }
  try {
    window.localStorage.setItem('access_token', accessToken);
    window.localStorage.setItem('new_api_access_token', accessToken);
  } catch (e) {}
  try {
    const secure = window.location.protocol === 'https:' ? '; Secure' : '';
    document.cookie = `access_token=${encodeURIComponent(accessToken)}; Path=/; SameSite=Strict${secure}`;
  } catch (e) {}
};

const resolveClawApiBase = (bootstrapData = {}) => {
  const candidates = [
    bootstrapData.api_base,
    bootstrapData.api_base_url,
    bootstrapData.backend_url,
    bootstrapData.backendUrl,
    CLAW_API_BASE,
  ];
  for (const candidate of candidates) {
    if (typeof candidate === 'string' && candidate.trim() !== '') {
      return candidate.trim();
    }
  }
  return CLAW_API_BASE;
};

const buildClawBootstrapContext = (bootstrapData = {}) => {
  const apiBase = resolveClawApiBase(bootstrapData);
  const accessToken =
    typeof bootstrapData.access_token === 'string'
      ? bootstrapData.access_token
      : '';

  return {
    apiBase,
    apiBaseUrl:
      typeof bootstrapData.api_base_url === 'string' &&
      bootstrapData.api_base_url.trim() !== ''
        ? bootstrapData.api_base_url.trim()
        : apiBase,
    backendUrl:
      typeof bootstrapData.backend_url === 'string' &&
      bootstrapData.backend_url.trim() !== ''
        ? bootstrapData.backend_url.trim()
        : typeof bootstrapData.backendUrl === 'string' &&
            bootstrapData.backendUrl.trim() !== ''
          ? bootstrapData.backendUrl.trim()
          : apiBase,
    clawBase:
      typeof bootstrapData.claw_base === 'string' &&
      bootstrapData.claw_base.trim() !== ''
        ? bootstrapData.claw_base.trim()
        : '/claw',
    frontendUI:
      typeof bootstrapData.frontend_ui === 'string' &&
      bootstrapData.frontend_ui.trim() !== ''
        ? bootstrapData.frontend_ui.trim()
        : '/console/claw',
    profile: bootstrapData.profile || bootstrapData.user || null,
    user: bootstrapData.user || bootstrapData.profile || null,
    modelConfig: bootstrapData.model_config || bootstrapData.models || null,
    models: bootstrapData.models || bootstrapData.model_config || null,
    accessToken,
    hasAccessToken: Boolean(bootstrapData.has_access_token || accessToken),
  };
};

const Claw = () => {
  const [phase, setPhase] = useState('loading');
  const [errorReason, setErrorReason] = useState('timeout');
  const [reloadKey, setReloadKey] = useState(0);
  const iframeRef = useRef(null);
  const bootstrapCacheRef = useRef(null);
  const bootstrapRequestRef = useRef(null);
  const iframeSrc = useMemo(
    () => `/claw?api_base=${encodeURIComponent(CLAW_API_BASE)}`,
    [],
  );

  const loadBootstrap = useCallback(async () => {
    if (bootstrapCacheRef.current) {
      return bootstrapCacheRef.current;
    }
    if (bootstrapRequestRef.current) {
      return bootstrapRequestRef.current;
    }

    bootstrapRequestRef.current = API.get('/api/claw/bootstrap', {
      skipErrorHandler: true,
    })
      .then((res) => {
        const context = buildClawBootstrapContext(res?.data?.data || {});
        bootstrapCacheRef.current = context;
        return context;
      })
      .catch(() => {
        const fallbackContext = buildClawBootstrapContext({});
        bootstrapCacheRef.current = fallbackContext;
        return fallbackContext;
      })
      .finally(() => {
        bootstrapRequestRef.current = null;
      });

    return bootstrapRequestRef.current;
  }, []);

  const injectBootstrapToIframe = useCallback(
    async (iframeElement) => {
      if (!iframeElement || !iframeElement.contentWindow) return;

      const frameWindow = iframeElement.contentWindow;
      const context = await loadBootstrap();

      try {
        frameWindow.__NEW_API_API_BASE__ = context.apiBase || CLAW_API_BASE;
        frameWindow.__NEW_API_CLAW_CONTEXT__ = context;
        frameWindow.__NEW_API_CLAW_BOOTSTRAP__ = context;
        if (context.accessToken) {
          frameWindow.__NEW_API_ACCESS_TOKEN__ = context.accessToken;
        }
      } catch (e) {
        return;
      }

      if (!context.accessToken) {
        return;
      }

      persistClawAccessToken(context.accessToken);

      try {
        const canAccessIframeStorage =
          frameWindow.location?.origin === window.location.origin;
        if (canAccessIframeStorage) {
          frameWindow.localStorage.setItem('access_token', context.accessToken);
          frameWindow.localStorage.setItem(
            'new_api_access_token',
            context.accessToken,
          );
        }
      } catch (e) {}
    },
    [loadBootstrap],
  );

  useEffect(() => {
    if (phase !== 'loading') return;
    const timer = window.setTimeout(() => {
      setErrorReason('timeout');
      setPhase('error');
    }, CLAW_LOAD_TIMEOUT_MS);
    return () => window.clearTimeout(timer);
  }, [phase, reloadKey]);

  useEffect(() => {
    void loadBootstrap();
  }, [loadBootstrap, reloadKey]);

  useEffect(() => {
    if (phase !== 'ready') return;
    void injectBootstrapToIframe(iframeRef.current);
  }, [phase, injectBootstrapToIframe, reloadKey]);

  const handleRetry = useCallback(() => {
    bootstrapCacheRef.current = null;
    bootstrapRequestRef.current = null;
    setErrorReason('timeout');
    setPhase('loading');
    setReloadKey((prev) => prev + 1);
  }, []);

  const handleLoad = useCallback(
    (event) => {
      setPhase('ready');
      void injectBootstrapToIframe(event.currentTarget);
    },
    [injectBootstrapToIframe],
  );

  const handleError = useCallback(() => {
    setErrorReason('load_error');
    setPhase('error');
  }, []);

  const isLoading = phase === 'loading';
  const hasError = phase === 'error';

  return (
    <div className='mt-[60px] px-2 h-[calc(100vh-76px)] h-[calc(100dvh-76px)] min-h-[420px]'>
      <div className='relative w-full h-full'>
        {isLoading && (
          <div className='absolute inset-0 z-10 flex items-center justify-center rounded-xl bg-[var(--semi-color-bg-0)]'>
            <Spin
              size='large'
              tip={<Typography.Text>Claw 加载中...</Typography.Text>}
            />
          </div>
        )}

        {hasError && (
          <div className='absolute inset-0 z-20 flex items-center justify-center rounded-xl bg-[var(--semi-color-bg-0)]'>
            <Empty
              title={
                errorReason === 'timeout' ? 'Claw 加载超时' : 'Claw 加载失败'
              }
              description={
                errorReason === 'timeout'
                  ? '请检查 Claw 前端服务或重试'
                  : '页面初始化异常，请稍后重试'
              }
            >
              <Button theme='solid' type='primary' onClick={handleRetry}>
                重试
              </Button>
            </Empty>
          </div>
        )}

        <iframe
          ref={iframeRef}
          key={reloadKey}
          src={iframeSrc}
          title='Claw'
          data-api-base={CLAW_API_BASE}
          onLoad={handleLoad}
          onError={handleError}
          style={{
            width: '100%',
            height: '100%',
            border: 'none',
            background: 'var(--semi-color-bg-0)',
            borderRadius: '12px',
            opacity: phase === 'ready' ? 1 : 0,
            transition: 'opacity 0.2s ease',
          }}
        />
      </div>
    </div>
  );
};

export default Claw;
