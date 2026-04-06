import client from "@/utils/client";

const DOCX_MIME =
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document";

const toText = (data: ArrayBuffer) => {
    try {
        return new TextDecoder("utf-8").decode(data);
    } catch {
        return "";
    }
};

const getHexSignature = (data: ArrayBuffer) => {
    const bytes = new Uint8Array(data.slice(0, 8));
    return Array.from(bytes)
        .map((b) => b.toString(16).padStart(2, "0"))
        .join(" ");
};

const isZip = (data: ArrayBuffer) => {
    const bytes = new Uint8Array(data.slice(0, 4));
    const sig = String.fromCharCode(...bytes);
    return sig === "PK\u0003\u0004" || sig === "PK\u0005\u0006" || sig === "PK\u0007\u0008";
};

export const getFile = async (
    file_id: string | number
): Promise<{blob: Blob; contentType: string; size: number; filename?: string}> => {
    const response = await client.get(`/contract/download/${file_id}`, {
        responseType: "arraybuffer",
        validateStatus: () => true,
        headers: {
            Accept: "application/octet-stream",
            Range: "bytes=0-",
        },
        transformResponse: (data) => data,
    });
    const status = response.status;
    if (status < 200 || status >= 300) {
        const text = toText(response.data as ArrayBuffer);
        throw new Error(text || `Download failed: ${status}`);
    }
    const contentType = String(response.headers?.["content-type"] || "");
    if (/application\/json|text\/html|text\/plain/i.test(contentType)) {
        const text = toText(response.data as ArrayBuffer);
        throw new Error(text || "Download failed");
    }
    const buffer = response.data as ArrayBuffer;
    if (!isZip(buffer)) {
        throw new Error(`下载内容不是DOCX: ${getHexSignature(buffer)}`);
    }
    const disposition = String(response.headers?.["content-disposition"] || "");
    const filenameMatch =
        /filename\*=UTF-8''([^;]+)|filename="([^"]+)"|filename=([^;]+)/i.exec(
            disposition
        );
    const rawFilename = filenameMatch?.[1] || filenameMatch?.[2] || filenameMatch?.[3];
    const filename = rawFilename ? decodeURIComponent(rawFilename.trim()) : undefined;
    const blob = new Blob([buffer], {type: contentType || DOCX_MIME});
    return {blob, contentType, size: buffer.byteLength, filename};
};
