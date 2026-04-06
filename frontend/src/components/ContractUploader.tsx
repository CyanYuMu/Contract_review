"use client";
import React, {useState} from "react";
import {Modal, UploadFile, UploadProps} from "antd";
import {Button, Upload} from "antd";
import toast from "react-hot-toast";
import {FileTextOutlined} from "@ant-design/icons";
import {upload as uploadApi} from "../lib/api/upload";
import {UploadStore} from "@/store/uploadStore";
import {createSession} from "@/lib/api/createSession";
import {TaskStore} from "@/store/taskStore";
import {assets} from "@/assets/assets";
import Image from "next/image";
import "./upload-progress.css";


type ContractUploaderProps = {
    onUploadSuccess?: () => void;
};

export default function ContractUploader({
                                             onUploadSuccess,
                                         }: ContractUploaderProps) {
    const [uploading, setUploading] = useState(false);
    const [progress, setProgress] = useState(0);
    const [fileList, setFileList] = useState<UploadFile[]>([]);
    const [uploadedFileName, setUploadedFileName] = useState<string | null>(null);
    const data = UploadStore((e) => e.data);
    const setData = UploadStore((e) => e.setData);
    const resetData = UploadStore((e) => e.resetData);
    const setTaskSessionId = TaskStore((e) => e.setData);
    const isDocxFile = (file: File | UploadFile) => {
        const fileName = "name" in file ? file.name : "";
        const extension = fileName.toLowerCase().split(".").pop();
        const mimeType = "type" in file ? file.type : "";
        return extension === "docx" || mimeType === "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
    };

    const beforeUpload = (file: File) => {
        if (!isDocxFile(file)) {
            Modal.error({
                title: "文件类型不支持",
                content: "仅支持上传.docx文件，请选择Word文档。",
            });
            return Upload.LIST_IGNORE;
        }
        return true;
    };

    const handleFileChange = async ({
                                        file,
                                        fileList: newFileList,
                                    }: {
        file: UploadFile;
        fileList: UploadFile[];
    }) => {
        setFileList(newFileList);
        if (file.status === "uploading" && file.percent !== undefined) {
            setUploading(true);
            setProgress(Math.round(file.percent));
        } else if (file.status === "done") {
            setUploadedFileName(file.name);
            setUploading(false);
            toast.success(`文件 "${file.name}" 上传成功`);
        } else if (file.status === "error") {
            setUploading(false);
            setProgress(0);
            setUploadedFileName(null);
          
        }
    };

    const handleCustomRequest: UploadProps["customRequest"] = async (options) => {
        const {file, onError, onSuccess,onProgress} = options;
        try {
            setUploading(true);
            setProgress(0);
            const formData = new FormData();
            const fileObj =
                (file as File) ||
                ("originFileObj" in (file as UploadFile)
                    ? (file as UploadFile).originFileObj
                    : undefined);
            if (fileObj instanceof File) {
                formData.append("file", fileObj);
            } else {
                throw new Error("无效的文件对象");
            }
            const uploadRes = await uploadApi(formData, (progressPercent) => {
                setProgress(progressPercent);
                onProgress?.({percent: progressPercent}, file);
            });

            // 检查上传是否真正成功
            if (!uploadRes || (!uploadRes.data && !uploadRes.file_url)) {
                throw new Error("上传失败：服务器未返回有效数据");
            }
            const title = (uploadRes?.data?.title ??
                uploadRes?.title ??
                (fileObj instanceof File ? fileObj.name : undefined)) as
                | string
                | undefined;
            const file_type = (uploadRes?.data?.file_type ??
                uploadRes?.file_type ??
                (fileObj instanceof File ? fileObj.type : undefined)) as
                | string
                | undefined;
            const file_url = (uploadRes?.data?.file_url ??
                uploadRes?.file_url ??
                uploadRes?.data?.url ??
                uploadRes?.url) as string | undefined;
            const partyA = uploadRes?.data?.party_a ?? uploadRes?.party_a;
            const partyB = uploadRes?.data?.party_b ?? uploadRes?.party_b;
            setData({title, file_type, file_url, party_a: partyA, party_b: partyB});
            const fileId =
                uploadRes?.data?.file_id ??
                uploadRes?.file_id ??
                uploadRes?.data?.id ??
                uploadRes?.id;
            TaskStore.getState().resetData();

            try {
                const sessionRes = await createSession({
                    title: title || uploadedFileName || "审阅",
                    session_type: "review",
                    file_id: fileId,
                });
                const sid = sessionRes.id;
                if (typeof sid === "number") {
                    setTaskSessionId(String(sid));
                } else {
                    console.error("创建会话失败：未返回有效的 session_id");
                }
            } catch (error) {
                console.error("创建会话失败：", error);
            }
            if (fileId) {
                localStorage.setItem("uploaded_file_id", String(fileId));
            }
            if (file_url) {
                localStorage.setItem("uploaded_file_url", file_url);
            }
            if (file_type) {
                localStorage.setItem("uploaded_file_type", file_type);
            }
            if (title) {
                localStorage.setItem("uploaded_file_title", title);
            }
            if (partyA !== undefined && partyA !== null) {
                localStorage.setItem("uploaded_party_a", partyA);
            } else {
                localStorage.removeItem("uploaded_party_a");
            }
            if (partyB !== undefined && partyB !== null) {
                localStorage.setItem("uploaded_party_b", partyB);
            } else {
                localStorage.removeItem("uploaded_party_b");
            }
            if (onSuccess) onSuccess({}, file);
            setTimeout(() => {
                if (onUploadSuccess) {
                    onUploadSuccess();
                }
            }, 100);
        } catch (err) {
            console.error("上传失败:", err);
            const errorMessage =
                err instanceof Error ? err.message : "文件上传失败，请重试";
            toast.error(errorMessage);
            if (onError) onError(err as Error);

            // 清理状态
            setUploading(false);
            setProgress(0);
            setFileList([]);
        }
    };

    const handleRemoveFile = () => {
        setFileList([]);
        setUploadedFileName(null);
        setProgress(0);
        resetData();
        TaskStore.getState().resetData();
        localStorage.removeItem("uploaded_file_id");
        localStorage.removeItem("uploaded_file_url");
        localStorage.removeItem("uploaded_file_type");
        localStorage.removeItem("uploaded_file_title");
        localStorage.removeItem("uploaded_party_a");
        localStorage.removeItem("uploaded_party_b");
    };

    return (
        <Upload
            name="file"
            data={{}}
            fileList={fileList}
            beforeUpload={beforeUpload}
            onChange={handleFileChange}
            onRemove={handleRemoveFile}
            customRequest={handleCustomRequest}
            accept=".docx"
            showUploadList={false}
        >
            {!uploadedFileName && !uploading ? (
                <div className="flex items-center gap-4 p-4">
                    <div className="flex-shrink-0">
                        <Image
                            src={assets.UploadFileIcon}
                            alt="上传文件"
                            width={64}
                            height={64}
                            style={{width: "9.9rem", height: "9.9rem"}}
                        />
                    </div>
                    <div className="flex flex-col gap-2  ml-[2.91rem]">
                        <Button
                            className="font-medium !bg-[#2260f2] !hover:bg-blue-600 !w-[14.5rem] !h-[3.75rem] !text-[1.5rem] !text-white  !tracking-[0.31em]">
                            上传合同文档
                        </Button>
                        <p className="text-[1.13rem] text-center font-normal">
                            支持Word合同文档
                        </p>
                    </div>
                </div>
            ) : uploading ? (
                <div className="upload-progress-container">
                    <div className="upload-progress-icon">
                        <svg className="upload-spinner" viewBox="0 0 50 50">
                            <circle
                                className="upload-spinner-circle"
                                cx="25"
                                cy="25"
                                r="20"
                                fill="none"
                                strokeWidth="4"
                            ></circle>
                        </svg>
                        <FileTextOutlined className="upload-file-icon"/>
                    </div>
                    <div className="upload-progress-info">
                        <div className="upload-progress-text">
              <span className="upload-filename">
                {uploadedFileName || "正在上传..."}
              </span>
                            <span className="upload-percent">{progress}%</span>
                        </div>
                        <div className="upload-progress-bar">
                            <div
                                className="upload-progress-fill"
                                style={{width: `${progress}%`}}
                            ></div>
                        </div>
                        <div className="upload-status-text">正在解析文档内容...</div>
                    </div>
                </div>
            ) : (
                <div className="flex items-center gap-4 p-4">
                    <div className="flex-shrink-0">
                        <Image
                            src={assets.UploadFileIcon}
                            alt="上传文件"
                            width={64}
                            height={64}
                            style={{width: "9.9rem", height: "9.9rem"}}
                        />
                    </div>
                    <div className="flex flex-col gap-2  ml-[2.91rem]">
                        <Button
                            className="font-medium !bg-[#2260f2] !hover:bg-blue-600 !w-[14.5rem] !h-[3.75rem] !text-[1.5rem] !text-white  !tracking-[0.31em]">
                            替换文件
                        </Button>
                        <div className="relative w-[14.5rem] group">
                            <p
                                className="text-[1.13rem] text-center font-normal truncate"
                                title={uploadedFileName ?? undefined}
                            >
                                {uploadedFileName}
                            </p>
                        </div>
                    </div>
                </div>
            )}
        </Upload>
    );
}