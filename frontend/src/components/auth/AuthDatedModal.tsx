'use client';

import React, { useEffect, useState } from 'react';
import { Modal, Button } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import { authDatedHandler } from '@/utils/authDatedHandler';

/**
 * 403 错误重新登录模态框组件
 * 当接口返回 403 状态时显示
 * 确保同一时间只弹出一个模态框
 */
export default function Auth403Modal() {
    const [visible, setVisible] = useState(false);

    useEffect(() => {
        // 订阅 403 错误事件
        const unsubscribe = authDatedHandler.subscribe(() => {
            setVisible(true);
        });

        // 组件卸载时取消订阅
        return () => {
            unsubscribe();
        };
    }, []);

    const handleClose = () => {
        setVisible(false);
        authDatedHandler.closeModal();
    };

    const handleConfirm = () => {
        setVisible(false);
        authDatedHandler.closeModal();
        // 触发登录模态框
        authDatedHandler.triggerLogin();
    };

    return (
        <Modal
            open={visible}
            onCancel={handleClose}
            footer={null}
            width={400}
            centered
            maskClosable={true}
            closable={true}
            zIndex={10002}
        >
            <div className="flex flex-col items-center py-6 px-4">
                <ExclamationCircleOutlined 
                    style={{ 
                        fontSize: '48px', 
                        color: '#faad14',
                        marginBottom: '16px'
                    }} 
                />
                <h3 className="text-lg font-semibold text-gray-800 mb-2">
                    登录已过期
                </h3>
                <p className="text-gray-500 text-center mb-6">
                    您的登录状态已过期，请重新登录
                </p>
                <Button
                    type="primary"
                    size="large"
                    onClick={handleConfirm}
                    className="w-32"
                >
                    去登陆
                </Button>
            </div>
        </Modal>
    );
}
