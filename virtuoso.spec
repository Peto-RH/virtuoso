Name:           virtuoso
Version:        0.1.0
Release:        1%{?dist}
Summary:        Virtual machine data collector for subscription management

License:        GPL-2.0-or-later
URL:            https://github.com/Peto-RH/virtuoso
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.23
BuildRequires:  libvirt-devel
BuildRequires:  make
BuildRequires:  systemd-rpm-macros

Requires:       subscription-manager
Requires:       group(libvirt)
Requires:       group(rhsm)

%description
Virtuoso collects virtual machine host-to-guest associations from hypervisor
environments and reports them to Red Hat subscription management platforms.

%prep
%autosetup

%build
%make_build build

%install
%make_build install_files \
    BUILDROOT=%{buildroot} \
    BINDIR=%{_bindir} \
    UNITDIR=%{_unitdir} \
    SYSUSERSDIR=%{_sysusersdir} \
    SYSCONFDIR=%{_sysconfdir}

%post
%systemd_post %{name}.timer

%preun
%systemd_preun %{name}.timer %{name}.service

%postun
%systemd_postun_with_restart %{name}.timer

%files
%{_bindir}/%{name}
%{_unitdir}/%{name}.service
%{_unitdir}/%{name}.timer
%{_sysusersdir}/%{name}.conf
%dir %attr(0750,root,virtuoso) %{_sysconfdir}/%{name}
%config(noreplace) %attr(0640,root,virtuoso) %{_sysconfdir}/%{name}/%{name}.toml
