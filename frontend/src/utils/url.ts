export const resolveFileUrl = (url?: string): string => {
    if (!url) return "";

    if (
        url.startsWith("/api/static/") ||
        url.startsWith("/uploads/") ||
        url.startsWith("uploads/") ||
        url.startsWith("/api/contract/download/") ||
        url.startsWith("/contract/download/")
    ) {
        return buildStaticFileUrl(url);
    }

    if (/^https?:\/\//i.test(url)) {
        const pathname = new URL(url).pathname;
        if (
            pathname.startsWith("/api/static/") ||
            pathname.startsWith("/uploads/") ||
            pathname.startsWith("/api/contract/download/") ||
            pathname.startsWith("/contract/download/")
        ) {
            return buildStaticFileUrl(url);
        }
        return url;
    }

    // Already a data, blob, or file URL
    if (/^(data:|blob:|file:)/i.test(url)) {
        return url;
    }

    const base = process.env.NEXT_PUBLIC_DEV_URL || "";

    if (!base) {
        return url;
    }

    if (base.endsWith("/") && url.startsWith("/")) {
        return `${base}${url.slice(1)}`;
    }

    if (!base.endsWith("/") && !url.startsWith("/")) {
        return `${base}/${url}`;
    }

    return `${base}${url}`;
};

export const buildStaticFileUrl = (fileUrl?: string): string => {
  if (!fileUrl) {
    throw new Error('fileUrl is required');
  }

  const getFrontendOrigin = () => {
    if (typeof window !== 'undefined') {
      return window.location.origin;
    }
    return process.env.NEXT_SERVER_URL || 'http://localhost:3000';
  };

  const toProxyUrl = (path: string) => {
    let proxyPath = path;
    if (proxyPath.startsWith('/api/static/')) {
      proxyPath = proxyPath.replace(/^\/api\/static(?=\/|$)/, '/api/proxy/static');
    } else if (proxyPath.startsWith('/api/contract/download/')) {
      proxyPath = proxyPath.replace(/^\/api(?=\/contract\/download\/)/, '/api/proxy');
    } else if (proxyPath.startsWith('/contract/download/')) {
      proxyPath = '/api/proxy' + proxyPath;
    } else if (proxyPath.startsWith('/uploads/')) {
      proxyPath = `/api/proxy/static${proxyPath.slice('/uploads'.length)}`;
    } else if (proxyPath.startsWith('uploads/')) {
      proxyPath = `/api/proxy/static/${proxyPath.slice('uploads/'.length)}`;
    }

    if (/^\/api\/proxy\/(static|contract\/download)\//.test(proxyPath)) {
      return getFrontendOrigin() + proxyPath;
    }

    return proxyPath;
  };

  if (/^https?:\/\//.test(fileUrl)) {
    try {
      const url = new URL(fileUrl);
      const path = url.pathname + url.search;
      const proxyUrl = toProxyUrl(path);
      return proxyUrl === path ? fileUrl : proxyUrl;
    } catch (error) {
      console.error('Failed to parse full URL:', { fileUrl, error });
      throw error;
    }
  }

  if (
    fileUrl.startsWith('/api/static/') ||
    fileUrl.startsWith('/api/contract/download/') ||
    fileUrl.startsWith('/contract/download/') ||
    fileUrl.startsWith('/uploads/') ||
    fileUrl.startsWith('uploads/')
  ) {
    return toProxyUrl(fileUrl);
  }

  // 不符合预期格式，原样返回或报错
  console.warn('Unexpected fileUrl format, returning as-is:', fileUrl);
  return fileUrl;
};
