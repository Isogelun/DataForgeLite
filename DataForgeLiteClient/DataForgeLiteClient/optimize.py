import os
import re


def optimize_xaml(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    # Define resources to inject
    resources = """    <Window.Resources>
        <!-- Colors -->
        <SolidColorBrush x:Key="PrimaryColor" Color="#FF2563EB"/>
        <SolidColorBrush x:Key="PrimaryHoverColor" Color="#FF1D4ED8"/>
        <SolidColorBrush x:Key="SuccessColor" Color="#FF059669"/>
        <SolidColorBrush x:Key="SuccessHoverColor" Color="#FF047857"/>
        <SolidColorBrush x:Key="BgColor" Color="#FFF3F6FB"/>
        <SolidColorBrush x:Key="CardBgColor" Color="#FFFFFFFF"/>
        <SolidColorBrush x:Key="TextColor" Color="#FF111827"/>
        <SolidColorBrush x:Key="MutedTextColor" Color="#FF6B7280"/>
        <SolidColorBrush x:Key="BorderColor" Color="#FFE5E7EB"/>

        <!-- Card Style -->
        <Style x:Key="CardStyle" TargetType="Border">
            <Setter Property="Background" Value="{StaticResource CardBgColor}"/>
            <Setter Property="BorderBrush" Value="{StaticResource BorderColor}"/>
            <Setter Property="BorderThickness" Value="1"/>
            <Setter Property="CornerRadius" Value="12"/>
            <Setter Property="Padding" Value="16"/>
            <Setter Property="Effect">
                <Setter.Value>
                    <DropShadowEffect BlurRadius="10" ShadowDepth="2" Color="#10000000" Opacity="0.05"/>
                </Setter.Value>
            </Setter>
        </Style>

        <!-- Modern Button Style -->
        <Style x:Key="ModernButton" TargetType="Button">
            <Setter Property="Background" Value="White"/>
            <Setter Property="Foreground" Value="{StaticResource TextColor}"/>
            <Setter Property="BorderBrush" Value="{StaticResource BorderColor}"/>
            <Setter Property="BorderThickness" Value="1"/>
            <Setter Property="Padding" Value="12,6"/>
            <Setter Property="Cursor" Value="Hand"/>
            <Setter Property="Template">
                <Setter.Value>
                    <ControlTemplate TargetType="Button">
                        <Border Background="{TemplateBinding Background}"
                                BorderBrush="{TemplateBinding BorderBrush}"
                                BorderThickness="{TemplateBinding BorderThickness}"
                                CornerRadius="6">
                            <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
                        </Border>
                        <ControlTemplate.Triggers>
                            <Trigger Property="IsMouseOver" Value="True">
                                <Setter Property="Background" Value="#FFF9FAFB"/>
                            </Trigger>
                            <Trigger Property="IsPressed" Value="True">
                                <Setter Property="Background" Value="#FFF3F4F6"/>
                            </Trigger>
                        </ControlTemplate.Triggers>
                    </ControlTemplate>
                </Setter.Value>
            </Setter>
        </Style>

        <Style x:Key="PrimaryButton" TargetType="Button" BasedOn="{StaticResource ModernButton}">
            <Setter Property="Background" Value="{StaticResource PrimaryColor}"/>
            <Setter Property="Foreground" Value="White"/>
            <Setter Property="BorderThickness" Value="0"/>
            <Setter Property="Template">
                <Setter.Value>
                    <ControlTemplate TargetType="Button">
                        <Border Background="{TemplateBinding Background}" CornerRadius="6">
                            <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
                        </Border>
                        <ControlTemplate.Triggers>
                            <Trigger Property="IsMouseOver" Value="True">
                                <Setter Property="Background" Value="{StaticResource PrimaryHoverColor}"/>
                            </Trigger>
                        </ControlTemplate.Triggers>
                    </ControlTemplate>
                </Setter.Value>
            </Setter>
        </Style>

        <Style x:Key="SuccessButton" TargetType="Button" BasedOn="{StaticResource PrimaryButton}">
            <Setter Property="Background" Value="{StaticResource SuccessColor}"/>
            <Setter Property="Template">
                <Setter.Value>
                    <ControlTemplate TargetType="Button">
                        <Border Background="{TemplateBinding Background}" CornerRadius="6">
                            <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
                        </Border>
                        <ControlTemplate.Triggers>
                            <Trigger Property="IsMouseOver" Value="True">
                                <Setter Property="Background" Value="{StaticResource SuccessHoverColor}"/>
                            </Trigger>
                        </ControlTemplate.Triggers>
                    </ControlTemplate>
                </Setter.Value>
            </Setter>
        </Style>

        <!-- Modern TabControl -->
        <Style TargetType="TabControl">
            <Setter Property="Background" Value="Transparent"/>
            <Setter Property="BorderThickness" Value="0"/>
        </Style>
        <Style TargetType="TabItem">
            <Setter Property="Background" Value="Transparent"/>
            <Setter Property="Foreground" Value="{StaticResource MutedTextColor}"/>
            <Setter Property="FontSize" Value="16"/>
            <Setter Property="FontWeight" Value="Medium"/>
            <Setter Property="Padding" Value="16,12"/>
            <Setter Property="BorderThickness" Value="0,0,0,2"/>
            <Setter Property="BorderBrush" Value="Transparent"/>
            <Setter Property="Cursor" Value="Hand"/>
            <Setter Property="Template">
                <Setter.Value>
                    <ControlTemplate TargetType="TabItem">
                        <Border Background="{TemplateBinding Background}"
                                BorderBrush="{TemplateBinding BorderBrush}"
                                BorderThickness="{TemplateBinding BorderThickness}"
                                Padding="{TemplateBinding Padding}">
                            <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
                        </Border>
                        <ControlTemplate.Triggers>
                            <Trigger Property="IsSelected" Value="True">
                                <Setter Property="Foreground" Value="{StaticResource PrimaryColor}"/>
                                <Setter Property="BorderBrush" Value="{StaticResource PrimaryColor}"/>
                            </Trigger>
                            <Trigger Property="IsMouseOver" Value="True">
                                <Setter Property="Foreground" Value="{StaticResource PrimaryColor}"/>
                            </Trigger>
                        </ControlTemplate.Triggers>
                    </ControlTemplate>
                </Setter.Value>
            </Setter>
        </Style>

        <!-- DataGrid Style -->
        <Style TargetType="DataGrid">
            <Setter Property="Background" Value="White"/>
            <Setter Property="BorderBrush" Value="{StaticResource BorderColor}"/>
            <Setter Property="BorderThickness" Value="1"/>
            <Setter Property="RowBackground" Value="White"/>
            <Setter Property="AlternatingRowBackground" Value="#FFF9FAFB"/>
            <Setter Property="HeadersVisibility" Value="Column"/>
            <Setter Property="GridLinesVisibility" Value="Horizontal"/>
            <Setter Property="HorizontalGridLinesBrush" Value="{StaticResource BorderColor}"/>
            <Setter Property="VerticalGridLinesBrush" Value="Transparent"/>
        </Style>
        <Style TargetType="DataGridColumnHeader">
            <Setter Property="Background" Value="#FFF3F4F6"/>
            <Setter Property="Foreground" Value="{StaticResource MutedTextColor}"/>
            <Setter Property="FontWeight" Value="SemiBold"/>
            <Setter Property="Padding" Value="12,10"/>
            <Setter Property="BorderThickness" Value="0,0,0,1"/>
            <Setter Property="BorderBrush" Value="{StaticResource BorderColor}"/>
        </Style>
        <Style TargetType="DataGridCell">
            <Setter Property="Padding" Value="12,8"/>
            <Setter Property="Template">
                <Setter.Value>
                    <ControlTemplate TargetType="DataGridCell">
                        <Border Padding="{TemplateBinding Padding}" Background="{TemplateBinding Background}">
                            <ContentPresenter VerticalAlignment="Center"/>
                        </Border>
                    </ControlTemplate>
                </Setter.Value>
            </Setter>
        </Style>
    </Window.Resources>
"""

    # Inject resources after <Window ...>
    window_end = content.find('>') + 1
    # Actually, <Window ... > might span multiple lines. Let's find the first <Grid>
    grid_start = content.find('<Grid>')
    
    # Add FontFamily to Window
    content = content.replace('Background="#FFF3F6FB">', 'Background="#FFF3F6FB"\n        FontFamily="Microsoft YaHei, Segoe UI, sans-serif">')
    
    content = content[:grid_start] + resources + content[grid_start:]

    # Replace Buttons
    # Primary Buttons
    content = re.sub(r'Background="#FF2563EB"[\s\n]*Foreground="White"[\s\n]*BorderBrush="#FF2563EB"', 'Style="{StaticResource PrimaryButton}"', content)
    # Success Buttons
    content = re.sub(r'Background="#FF059669"[\s\n]*Foreground="White"[\s\n]*BorderBrush="#FF059669"', 'Style="{StaticResource SuccessButton}"', content)
    
    # Normal Buttons: find all <Button ...> and add Style="{StaticResource ModernButton}" if not Primary or Success
    def button_replacer(match):
        btn = match.group(0)
        if 'Style=' not in btn:
            return btn.replace('<Button ', '<Button Style="{StaticResource ModernButton}" ')
        return btn
    content = re.sub(r'<Button[^>]*>', button_replacer, content)

    # Replace Cards
    card_pattern = r'Background="#FFF8FAFC"[\s\n]*BorderBrush="#FFE5E7EB"[\s\n]*BorderThickness="1"[\s\n]*CornerRadius="12"[\s\n]*Padding="16"'
    content = re.sub(card_pattern, 'Style="{StaticResource CardStyle}"', content)

    # Clean up DataGrid inline styles
    content = re.sub(r'HeadersVisibility="Column"[\s\n]*GridLinesVisibility="Horizontal"[\s\n]*RowHeight="\d+"[\s\n]*AlternatingRowBackground="#FFF9FAFB"', '', content)
    content = re.sub(r'RowHeight="\d+"[\s\n]*AlternatingRowBackground="#FFF9FAFB"', '', content)

    # Clean up TabControl inline styles
    content = re.sub(r'Background="White"[\s\n]*BorderThickness="0"', '', content)

    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(content)

if __name__ == "__main__":
    # 使用脚本所在目录的 MainWindow.xaml，便于任意环境运行
    script_dir = os.path.dirname(os.path.abspath(__file__))
    xaml_path = os.path.join(script_dir, "MainWindow.xaml")
    optimize_xaml(xaml_path)
