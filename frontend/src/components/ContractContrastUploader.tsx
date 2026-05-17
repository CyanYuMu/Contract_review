"use client";
import React, { useState } from "react";
import {Modal, UploadFile, UploadProps} from "antd";
import { Button, Upload } from "antd";
import toast from "react-hot-toast";

import { upload as uploadApi } from "../lib/api/upload";
import { ContrastuploadStore } from "@/store/ContrastuploadStore";
import { assets } from "@/assets/assets";
import Image from "next/image";
import "./upload-progress.css";

export type UploadedContrastFile = {
  title: string;
  file_type: string;
  file_url: string;
  file_id: number;
};

type ContractContrastUploaderProps = {
  onUploadSuccess?: (file: UploadedContrastFile) => void;
  label?: string;
  isOriginal?: boolean;
};

export default function ContractContrastUploader({
  onUploadSuccess,
  label = "标准文档上传",
  isOriginal = true,
}: ContractContrastUploaderProps) {
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [uploadedFileName, setUploadedFileName] = useState<string | null>(null);
  const [uploadingFileName, setUploadingFileName] = useState<string | null>(null);

  const { setOriginalFile, setComparisonFile } = ContrastuploadStore();

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
    fileList: newFileList,
  }: { file: UploadFile; fileList: UploadFile[] }) => {
    setFileList(newFileList);
  };

  const handleCustomRequest: UploadProps["customRequest"] = async (options) => {
    const { file, onError, onSuccess } = options;
    try {
      const fileObj = (file as File) || ("originFileObj" in (file as UploadFile) ? (file as UploadFile).originFileObj : undefined);
      if (!(fileObj instanceof File)) {
        throw new Error("无效的文件对象");
      }

      setUploadingFileName(fileObj.name);
      setUploading(true);
      setProgress(0);

      const formData = new FormData();
      formData.append("file", fileObj);

      const uploadResult = await uploadApi(formData, (progressPercent) => {
        setProgress(progressPercent);
      });

      const file_id = uploadResult.data.file_id;
      const title = uploadResult.data.title || fileObj.name;
      const file_type = uploadResult.data.file_type || fileObj.type;

      const reader = new FileReader();
      reader.onload = () => {
        const file_url = reader.result as string;

     
        if (isOriginal) {
          setOriginalFile({
            title,
            file_type,
            file_url,
            file_id
          });
          localStorage.setItem("original_file_url", file_url);
          localStorage.setItem("original_file_title", title);
          localStorage.setItem("original_file_type", file_type);
          localStorage.setItem("original_file_id", String(file_id));
        } else {
          setComparisonFile({
            title,
            file_type,
            file_url,
            file_id
          });
          localStorage.setItem("comparison_file_url", file_url);
          localStorage.setItem("comparison_file_title", title);
          localStorage.setItem("comparison_file_type", file_type);
          localStorage.setItem("comparison_file_id", String(file_id));
        }

        if (onSuccess) onSuccess({}, file);
        if (onUploadSuccess) {
          onUploadSuccess({
            title,
            file_type,
            file_url,
            file_id,
          });
        }
      };
      reader.onerror = () => {
        toast.error("文件读取失败");
        if (onError) onError(new Error("文件读取失败"));
        setUploading(false);
        setProgress(0);
      };
      reader.readAsDataURL(fileObj);

      setUploadedFileName(title);
      setProgress(100);
      setTimeout(() => {
        setUploading(false);
        setUploadingFileName(null);
        toast.success(`文件 "${title}" 上传成功`);
      }, 500);
    } catch (err) {
      const errorMessage = (err as Error).message || "文件上传失败，请重试";
      toast.error(errorMessage);
      if (onError) onError(err as Error);
      setUploading(false);
      setProgress(0);
      setFileList([]);
      setUploadingFileName(null);
    }
  };

  const handleRemoveFile = () => {
    setFileList([]);
    setUploadedFileName(null);
    setProgress(0);

    if (isOriginal) {
      localStorage.removeItem("original_file_id");
      localStorage.removeItem("original_file_title");
      localStorage.removeItem("original_file_type");
      localStorage.removeItem("original_file_url");
      ContrastuploadStore.getState().setOriginalFile({});
    } else {
      localStorage.removeItem("comparison_file_id");
      localStorage.removeItem("comparison_file_title");
      localStorage.removeItem("comparison_file_type");
      localStorage.removeItem("comparison_file_url");
      ContrastuploadStore.getState().setComparisonFile({});
    }
  };

  return (
    <>
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
          <div className="flex flex-col items-center justify-center gap-4 p-4">
            <div className="flex-shrink-0">
              <Image
                src={assets.UploadFileIcon}
                alt="上传文件"
                width={180}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Button
                className="font-medium !bg-[#2260f2] !hover:bg-blue-600 !w-[14.5rem] !h-[3.75rem] !text-[1.5rem] !text-white !tracking-[0.31em]"
              >
                上传{label}
              </Button>
              <p className="text-[1.13rem] text-center font-normal">
                支持Word合同文档
              </p>
            </div>
          </div>
        ) : uploading ? (
          <div className="flex flex-col items-center p-4 w-full">
            <div className="flex items-center gap-3 mb-2">
              <div className="text-blue-500">
                <Image
                  src={assets.UploadIcon2}
                  alt="上传文件"
                  width={24}
                  height={24}
                />
              </div>
              <span className="text-gray-700 text-base">
                {uploadingFileName || "文件上传中"}
              </span>
            </div>

            <div className="flex items-center mb-4">
              <div className="w-[200px] h-2 bg-rgb[(245, 248, 255, 1)] rounded-full overflow-hidden">
                <div
                  className="h-full rounded-full transition-all duration-300 ease-linear"
                  style={{ width: `${progress}%`, backgroundColor: 'rgba(34, 96, 242, 1)' }}
                ></div>
              </div>
              <span className="ml-2 text-base">
                {progress}%
              </span>
            </div>
            <div className="w-[160px] flex justify-center">
              <button
                type="button"
                onClick={() => {
                  handleRemoveFile();
                  setUploadingFileName(null);
                }}
                style={{
                  width: '136px',
                  height: '43px',
                  opacity: 1,
                  borderRadius: '5px',
                  border: '1px solid rgba(34, 96, 242, 1)',
                  display: 'flex',
                  flexDirection: 'column',
                  justifyContent: 'center',
                  alignItems: 'center',
                  padding: '8px 32px',
                  color: 'rgba(34, 96, 242, 1)'
                }}
              >
                重新上传
              </button>
            </div>
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center gap-4 p-4">
            <div className="flex flex-row items-center justify-center">
              <div className="text-blue-500">
                <Image
                  src={assets.UploadSuccessIcon}
                  alt="上传文件成功"
                  width={24}
                  height={24}
                />
              </div>
              <div className="relative group ml-2">
                <p
                  className="text-[1.13rem] text-center font-normal truncate"
                  title={uploadedFileName ?? undefined}
                >
                  {uploadedFileName}
                </p>
              </div>
            </div>
            <Button
              style={{
                width: '136px',
                height: '43px',
                opacity: 1,
                borderRadius: '5px',
                border: '1px solid rgba(34, 96, 242, 1)',
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center',
                padding: '8px 32px',
                color: 'rgba(34, 96, 242, 1)'
              }}
              onClick={handleRemoveFile}
            >
              重新上传文件
            </Button>
          </div>
        )}
      </Upload>
    </>
  );
}
