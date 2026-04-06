import React from "react";
import Image from "next/image";
import {assets} from "@/assets/assets";
import type {User} from "@/lib/Interface";
import TopbarTabs, {type TabType} from "./TopbarTabs";

type TopbarProps = {
    user?: User | null;
    onLoginClick?: () => void;
    onLogoutClick?: () => void;
    activeTab?: TabType | null;
};

export default function Topbar({
                                   user,
                                   onLoginClick,
                                   onLogoutClick,
                                   activeTab,
                               }: TopbarProps) {
    return (
        <div
            className="bg-[#2260f2] h-[4rem] text-white flex flex-row"
            style={{alignItems: "flex-end"}}
        >
            <div
                className="flex flex-row gap-5 items-center flex-1"
                style={{paddingBottom: "0.5rem"}}
            >
                <Image
                    src={assets.CquptIcon}
                    alt="重庆邮电大学"
                    className="ml-[0.44rem]"
                />
                <span className="text-[1.5rem] font-medium text-[#FFFAFA]">
          AI智审·合同审阅助理
        </span>
            </div>
            <div
                className="mr-[1.63rem]"
                style={{marginBottom: 0, paddingBottom: 0}}
            >
                <TopbarTabs
                    user={user}
                    onLoginClick={onLoginClick}
                    onLogoutClick={onLogoutClick}
                    activeTab={activeTab}
                />
            </div>
        </div>
    );
}
