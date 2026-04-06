'use client';

import React from 'react';
import { Pagination } from 'antd';
import type { PaginationProps } from 'antd';

interface CustomPaginationProps {
    /** 当前页码 */
    current: number;
    /** 每页条数 */
    pageSize: number;
    /** 数据总数 */
    total: number;
    /** 页码或 pageSize 改变的回调 */
    onChange: (page: number, pageSize: number) => void;
    /** 是否显示总数 */
    showTotal?: boolean;
    /** 总数文本自定义 */
    totalText?: (total: number) => string;
    /** 是否可以改变 pageSize */
    showSizeChanger?: boolean;
    /** 是否可以快速跳转至某页 */
    showQuickJumper?: boolean;
    /** 指定每页可以显示多少条 */
    pageSizeOptions?: number[];
    /** 自定义类名 */
    className?: string;
}

/**
 * 自定义分页组件
 * 统一的分页样式和配置
 */
export const CustomPagination: React.FC<CustomPaginationProps> = ({
    current,
    pageSize,
    total,
    onChange,
    showTotal = true,
    totalText = (total) => `共 ${total} 条`,
    showSizeChanger = true,
    showQuickJumper = true,
    pageSizeOptions = [10, 20, 50, 100],
    className = ''
}) => {
    // 分页本地化配置
    const paginationLocale = {
        jump_to: '跳至',
        page: '页',
        items_per_page: '条/页',
    };

    return (
        <div className={`border-t border-gray-200 flex justify-end py-2 bg-white ${className}`}>
            <div className="flex items-center gap-4">
                {showTotal && (
                    <span className="text-sm text-gray-600">
                        {totalText(total)}
                    </span>
                )}
                <Pagination
                    current={current}
                    pageSize={pageSize}
                    total={total}
                    onChange={onChange}
                    showSizeChanger={showSizeChanger}
                    showQuickJumper={showQuickJumper}
                    pageSizeOptions={pageSizeOptions}
                    locale={paginationLocale}
                />
            </div>
        </div>
    );
};