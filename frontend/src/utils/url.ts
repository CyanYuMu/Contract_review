export const resolveFileUrl = (url?: string): string => {
    if (!url) return "";

    // Already an absolute or data URL
    if (/^(https?:|data:|blob:|file:)/i.test(url)) {
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

 
  if (/^https?:\/\//.test(fileUrl)) {
    console.log("进入https")
    try {
      const url = new URL(fileUrl);
      const path = url.pathname + url.search;
      const proxyPath = path.replace(/^\/api\/static(?=\/| $ )/, '/api/proxy/static');

      let origin: string;
      if (typeof window !== 'undefined') {
        origin = window.location.origin;
      } 
       else {
        origin = process.env.NEXT_PUBLIC_DEV_URL || 'http://localhost:3000';
      }
      return origin + proxyPath;
    } catch (error) {
      console.error('Failed to parse full URL:', { fileUrl, error });
      throw error;
    }
  }


  if (fileUrl.startsWith('/api/static/')) {
        console.log("进入/api/static/")
        const proxyPath = fileUrl.replace(/^\/api\/static(?=\/| $ )/, '/api/proxy/static');

    // 客户端：使用当前 origin
    if (typeof window !== 'undefined') {
      return window.location.origin + proxyPath;
    }


    // fallback
    const fallback = process.env.NEXT_PUBLIC_DEV_URL || '';
    if (!fallback) {

      throw new Error(
        'Cannot determine origin: provide `req` in server context or set NEXT_PUBLIC_DEV_URL'
      );
    }
    return fallback + proxyPath;
  }

  // 不符合预期格式，原样返回或报错
  console.warn('Unexpected fileUrl format, returning as-is:', fileUrl);
  return fileUrl;
};

